package main

import (
	"fmt"
	"strings"

	agentplugin "github.com/WangYihang/Platypus/internal/agent/plugin"
)

// Per-capability rendering is structured as four parallel
// registries keyed by agentplugin.CapabilityID instead of
// switch-on-string blocks. Three properties drop out of this:
//
//   - the compiler catches a typo'd CapabilityID at the call site
//     (it's a defined string-derived type, not an arbitrary string)
//   - adding a new capability only requires adding a constant in
//     internal/agent/plugin/manifest.go AND an entry in the
//     authority list AllCapabilityIDs; init() at the bottom of
//     this file panics on startup if any of these registries
//     misses one, before any user-facing CLI runs
//   - the YAML / SDK-hint / description text for a single family
//     stays grep-able to one map entry instead of scattered
//     across four switch arms

// capYAMLEntries renders the YAML fragment for one family inside
// the manifest's `capabilities:` block. CapLog is the empty string
// because log is implicit (every plugin gets it without a
// manifest entry); the renderer drops empty entries before
// joining.
var capYAMLEntries = map[agentplugin.CapabilityID]string{
	agentplugin.CapLog:       "",
	agentplugin.CapKV:        "  kv: true",
	agentplugin.CapSysInfo:   "  sysinfo: true",
	agentplugin.CapExec:      "  exec:\n    commands:\n      - /usr/bin/echo",
	agentplugin.CapFSRead:    "  fs.read:\n    paths:\n      - /etc",
	agentplugin.CapFSWrite:   "  fs.write:\n    paths:\n      - /tmp/{{.ID}}-scratch",
	agentplugin.CapNetHTTP:   "  net.http:\n    hosts:\n      - example.com",
	agentplugin.CapProcess:   "  process:\n    commands:\n      - /bin/sh",
	agentplugin.CapNetDial:   "  net.dial:\n    targets:\n      - 127.0.0.1:8080",
	agentplugin.CapNetListen: "  net.listen:\n    binds:\n      - 127.0.0.1:1080",
}

// capHints is the per-family commented-out example snippet the
// Rust scaffold drops into lib.rs so authors have a one-line lift
// from "I picked fs.read" to "here's how to call it". CapLog has
// no hint — the scaffold's main RPC already calls info!() so log
// is exercised by default.
var capHints = map[agentplugin.CapabilityID]string{
	agentplugin.CapLog:       "",
	agentplugin.CapKV:        "// kv: var::set(\"counter\", 1)?;  let v: Option<i64> = var::get(\"counter\")?;",
	agentplugin.CapSysInfo:   "// sysinfo: let info = host::get_sys_info()?;  // os, arch fields",
	agentplugin.CapExec:      "// exec: let out = host::exec(\"/usr/bin/echo\", &[\"hello\"])?;",
	agentplugin.CapFSRead:    "// fs.read: let bytes = host::fs_read(\"/etc/hostname\")?;",
	agentplugin.CapFSWrite:   "// fs.write: host::fs_write(\"/tmp/scratch/x\", b\"hi\")?;",
	agentplugin.CapNetHTTP:   "// net.http: let req = HttpRequest::new(\"https://example.com/\");\n// let resp = http::request::<()>(&req, None)?;",
	agentplugin.CapProcess:   "// process: let pid = host::process_spawn(\"/bin/sh\", &[\"-c\", \"echo hi\"])?;",
	agentplugin.CapNetDial:   "// net.dial: let conn = host::net_dial(\"127.0.0.1:8080\")?;",
	agentplugin.CapNetListen: "// net.listen: let lst = host::net_listen(\"127.0.0.1:1080\")?;",
}

// capDescriptions mirrors the operator-facing copy in
// desktop/frontend/src/lib/capabilities.ts so the generated
// README's per-family description matches what the operator sees
// in the install dialog. Keep these in sync if either side
// changes — the init() check below catches the case where a new
// capability is added but a description is missed.
var capDescriptions = map[agentplugin.CapabilityID]string{
	agentplugin.CapLog:       "structured log output to the agent's per-plugin ring buffer",
	agentplugin.CapKV:        "namespaced key-value store (the plugin's own scope only)",
	agentplugin.CapSysInfo:   "read-only host snapshot (os/arch/hostname)",
	agentplugin.CapExec:      "execute commands from a host-side allowlist",
	agentplugin.CapFSRead:    "read files from a host-side path allowlist",
	agentplugin.CapFSWrite:   "write files inside a host-side path allowlist",
	agentplugin.CapNetHTTP:   "make outbound HTTP requests to a host-side allowlist",
	agentplugin.CapProcess:   "spawn an interactive PTY process (operator approval required)",
	agentplugin.CapNetDial:   "open outbound TCP connections to a host-side target allowlist",
	agentplugin.CapNetListen: "bind a TCP listener at a host-side bind allowlist",
}

// init asserts that every capability the agent recognises
// (agentplugin.AllCapabilityIDs) has an entry in every renderer
// registry. A future addition to the CapabilityID constants must
// also land here, or `platypus-cli` panics on startup with a
// list of the missing ones — much louder than a silent default-
// arm switch fall-through.
func init() {
	registries := []struct {
		name string
		m    map[agentplugin.CapabilityID]string
	}{
		{"capYAMLEntries", capYAMLEntries},
		{"capHints", capHints},
		{"capDescriptions", capDescriptions},
	}
	var missing []string
	for _, r := range registries {
		for _, c := range agentplugin.AllCapabilityIDs {
			if _, ok := r.m[c]; !ok {
				missing = append(missing, fmt.Sprintf("%s missing %q", r.name, c))
			}
		}
	}
	if len(missing) > 0 {
		panic("plugin_new_caps: capability registries out of sync with agentplugin.AllCapabilityIDs:\n  " +
			strings.Join(missing, "\n  "))
	}
}

// renderCapabilitiesYAML joins per-family fragments into the
// `capabilities:` block body. Empty input → empty output, and
// the template uses {{- if .CapabilitiesYAML }} to skip the
// section header entirely (no `capabilities:` line at all). That
// matches the manifest validator's rule: an empty capabilities
// map and an absent capabilities key are both valid.
func renderCapabilitiesYAML(families []agentplugin.CapabilityID) string {
	var parts []string
	for _, f := range families {
		entry := capYAMLEntries[f]
		if entry != "" {
			parts = append(parts, entry)
		}
	}
	return strings.Join(parts, "\n")
}

// renderCapHints joins the per-family hint snippets, prefixed
// with the family name as a comment so the author can scan to the
// one they're implementing.
func renderCapHints(families []agentplugin.CapabilityID) string {
	var parts []string
	for _, f := range families {
		snippet := capHints[f]
		if snippet == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("// --- %s ---\n%s", f, snippet))
	}
	return strings.Join(parts, "\n\n")
}

// renderCapabilityList renders the README's bulleted capability
// list. log is annotated as "(implicit)" when present so the
// operator-facing description matches what they'll actually see
// in the install dialog.
func renderCapabilityList(families []agentplugin.CapabilityID) string {
	if len(families) == 0 {
		return "- (none — only the implicit `log` capability)"
	}
	var parts []string
	for _, f := range families {
		descr := capDescriptions[f]
		if f == agentplugin.CapLog {
			parts = append(parts, fmt.Sprintf("- `%s` — %s (implicit, every plugin)", f, descr))
		} else {
			parts = append(parts, fmt.Sprintf("- `%s` — %s", f, descr))
		}
	}
	return strings.Join(parts, "\n")
}
