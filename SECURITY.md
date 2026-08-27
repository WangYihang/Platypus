# Security Policy

## Supported versions

Fixes land on the latest minor release. Older minors are not backported.

| Version | Supported          |
| ------- | ------------------ |
| 1.5.x   | :white_check_mark: |
| < 1.5   | :x:                |

## Reporting a vulnerability

Report privately through GitHub, not in a public issue:

**[Open a draft security advisory](https://github.com/WangYihang/Platypus/security/advisories/new)**
(Security → Advisories → Report a vulnerability)

Useful to include, roughly in order of how much it helps:

- Which component — server, agent, desktop app, or CLI — and the version
  or commit.
- What an attacker gains, and what access they need to start. "Any host
  on the network" and "an authenticated project viewer" are very
  different reports.
- A reproduction. A failing test against `internal/` is ideal; a curl
  sequence or a short script is fine.

You should get an acknowledgement within a few days. This is a
small project, so please read that as best-effort rather than a
guaranteed SLA. If a report goes unanswered for two weeks, feel free to
ping the advisory thread.

Please give a fix a reasonable window before publishing. If you plan to
disclose on a fixed date, say so in the first message so the timeline is
shared from the start.

## What counts as a vulnerability here

Platypus is a remote administration tool. Running commands on a managed
host, reading its files, and opening tunnels through it are the product,
not a flaw — a report that an authenticated operator can get a shell
will be closed as working-as-intended.

What we do want to hear about is anything that breaks the boundaries the
system is supposed to hold:

- **Authentication and enrollment** — registering an agent without a
  valid enrollment token, replaying or forging one, or bypassing the
  approval queue.
- **Transport** — defeating TLS verification between agent and server,
  agent impersonation, or MITM against the yamux link.
- **Authorization** — crossing a project boundary, or a viewer or
  operator reaching something scoped to admins. The signed preview-token
  scheme for file reads is in scope, including token forgery or
  substitution.
- **Privilege escalation on the host** — the agent gaining more than it
  was installed with, or a plugin escaping the WASM sandbox or its
  declared capability set.
- **Secret disclosure** — leaking the CA key, the KEK, enrollment
  tokens, or session credentials through logs, API responses, or
  recordings.
- **Supply chain** — signature verification bypass in plugin install or
  update, or in the release artifacts.

Denial of service against your own server by an operator you already
trusted is generally out of scope. Findings from automated scanners are
welcome, but please confirm they are reachable in this codebase before
filing.

## Operating Platypus safely

A few defaults worth knowing about, since they affect your exposure more
than most bugs would:

- **`PLATYPUS_CA_KEK`** — set it in production. Without it the server
  auto-generates a KEK next to the data volume, so anyone who can read
  the volume can decrypt the CA key. The server logs a warning when it
  falls back.
- **Server exposure** — the agent listener terminates TLS and expects to
  face the network, but the admin API and web UI do not need to. Keep
  them behind your own boundary.
- **AI session summaries** — off by default, per project. Turning them
  on sends redacted terminal output to whatever endpoint
  `PLATYPUS_LLM_BASE_URL` names. Redaction is best-effort by
  construction (see `internal/llm/redact.go`); treat it as reducing
  accidental exposure, not as a guarantee.
