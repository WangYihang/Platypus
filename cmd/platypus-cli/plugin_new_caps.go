package main

import (
	"fmt"
	"strings"
)

// capYAMLEntry renders one capability family into the YAML
// fragment that goes under `capabilities:`. Allowlist-bearing caps
// (exec, fs.read, fs.write, net.http, process, net.dial,
// net.listen) get a placeholder list the author edits; the two
// boolean caps (kv, sysinfo) render as `kv: true`. CapLog is
// implicit — every plugin gets it without a manifest entry — so we
// drop it on the floor here.
//
// Indentation is two spaces for the family key (already inside
// `capabilities:`), four for the nested mapping fields. Matches
// the style the existing example/plugins manifests use.
func capYAMLEntry(family string) string {
	switch family {
	case "log":
		return ""
	case "kv":
		return "  kv: true"
	case "sysinfo":
		return "  sysinfo: true"
	case "exec":
		return "  exec:\n    commands:\n      - /usr/bin/echo"
	case "fs.read":
		return "  fs.read:\n    paths:\n      - /etc"
	case "fs.write":
		return "  fs.write:\n    paths:\n      - /tmp/{{.ID}}-scratch"
	case "net.http":
		return "  net.http:\n    hosts:\n      - example.com"
	case "process":
		return "  process:\n    commands:\n      - /bin/sh"
	case "net.dial":
		return "  net.dial:\n    targets:\n      - 127.0.0.1:8080"
	case "net.listen":
		return "  net.listen:\n    binds:\n      - 127.0.0.1:1080"
	default:
		return ""
	}
}

// renderCapabilitiesYAML joins the per-family fragments into the
// `capabilities:` block body. Empty input → empty output, and the
// template uses {{- if .CapabilitiesYAML }} to skip the section
// header entirely (no `capabilities:` line at all). That matches
// the manifest validator's rule: an empty capabilities map and an
// absent capabilities key are both valid.
func renderCapabilitiesYAML(families []string) string {
	var parts []string
	for _, f := range families {
		entry := capYAMLEntry(f)
		if entry != "" {
			parts = append(parts, entry)
		}
	}
	return strings.Join(parts, "\n")
}

// capHintRust returns a multi-line code snippet showing how to call
// the granted capability via extism-pdk. Inserted into the Rust
// scaffold's lib.rs as a commented-out example so the author has a
// one-step lift from "I picked fs.read" to "here's how to call it".
func capHintRust(family string) string {
	switch family {
	case "kv":
		return "// kv: var::set(\"counter\", 1)?;  let v: Option<i64> = var::get(\"counter\")?;"
	case "sysinfo":
		return "// sysinfo: let info = host::get_sys_info()?;  // os, arch fields"
	case "exec":
		return "// exec: let out = host::exec(\"/usr/bin/echo\", &[\"hello\"])?;"
	case "fs.read":
		return "// fs.read: let bytes = host::fs_read(\"/etc/hostname\")?;"
	case "fs.write":
		return "// fs.write: host::fs_write(\"/tmp/scratch/x\", b\"hi\")?;"
	case "net.http":
		return "// net.http: let req = HttpRequest::new(\"https://example.com/\");\n// let resp = http::request::<()>(&req, None)?;"
	case "process":
		return "// process: let pid = host::process_spawn(\"/bin/sh\", &[\"-c\", \"echo hi\"])?;"
	case "net.dial":
		return "// net.dial: let conn = host::net_dial(\"127.0.0.1:8080\")?;"
	case "net.listen":
		return "// net.listen: let lst = host::net_listen(\"127.0.0.1:1080\")?;"
	default:
		return ""
	}
}

// capHintGo is the parallel snippet for the Go (TinyGo) scaffold.
// Emitted inside a /* ... */ block in main.go so the example
// compiles cleanly without the author having imported the host
// helpers — they uncomment as they reach for each capability.
func capHintGo(family string) string {
	switch family {
	case "kv":
		return "platypus.HostKVSet(\"counter\", []byte(\"1\"))\nv, _ := platypus.HostKVGet(\"counter\")"
	case "sysinfo":
		return "info, _ := platypus.HostSysInfo()  // info.OS, info.Arch"
	case "exec":
		return "out, _ := platypus.HostExec(\"/usr/bin/echo\", []string{\"hello\"})"
	case "fs.read":
		return "bytes, _ := platypus.HostFSRead(\"/etc/hostname\")"
	case "fs.write":
		return "platypus.HostFSWrite(\"/tmp/scratch/x\", []byte(\"hi\"))"
	case "net.http":
		return "resp, _ := platypus.HostHTTP(\"GET\", \"https://example.com/\", nil)"
	case "process":
		return "pid, _ := platypus.HostProcessSpawn(\"/bin/sh\", []string{\"-c\", \"echo hi\"})"
	case "net.dial":
		return "conn, _ := platypus.HostNetDial(\"127.0.0.1:8080\")"
	case "net.listen":
		return "lst, _ := platypus.HostNetListen(\"127.0.0.1:1080\")"
	default:
		return ""
	}
}

// renderCapHints joins the per-family hint snippets, prefixed with
// the family name as a comment so the author can scan to the one
// they're implementing. lang selects the SDK syntax.
func renderCapHints(families []string, lang string) string {
	var parts []string
	for _, f := range families {
		var snippet string
		switch lang {
		case "rust":
			snippet = capHintRust(f)
		case "go":
			snippet = capHintGo(f)
		}
		if snippet == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("// --- %s ---\n%s", f, snippet))
	}
	return strings.Join(parts, "\n\n")
}

// renderCapabilityList renders the README's bulleted capability
// list. log is rendered as "(implicit)" when present so the
// operator-facing description matches what they'll actually see in
// the install dialog.
func renderCapabilityList(families []string) string {
	if len(families) == 0 {
		return "- (none — only the implicit `log` capability)"
	}
	var parts []string
	for _, f := range families {
		descr := capDescription(f)
		if f == "log" {
			parts = append(parts, fmt.Sprintf("- `%s` — %s (implicit, every plugin)", f, descr))
		} else {
			parts = append(parts, fmt.Sprintf("- `%s` — %s", f, descr))
		}
	}
	return strings.Join(parts, "\n")
}

// capDescription mirrors desktop/frontend/src/lib/capabilities.ts
// so the README's per-family description matches what the operator
// sees in the install dialog. Keep these in sync if either side
// changes.
func capDescription(family string) string {
	switch family {
	case "log":
		return "structured log output to the agent's per-plugin ring buffer"
	case "kv":
		return "namespaced key-value store (the plugin's own scope only)"
	case "sysinfo":
		return "read-only host snapshot (os/arch/hostname)"
	case "exec":
		return "execute commands from a host-side allowlist"
	case "fs.read":
		return "read files from a host-side path allowlist"
	case "fs.write":
		return "write files inside a host-side path allowlist"
	case "net.http":
		return "make outbound HTTP requests to a host-side allowlist"
	case "process":
		return "spawn an interactive PTY process (operator approval required)"
	case "net.dial":
		return "open outbound TCP connections to a host-side target allowlist"
	case "net.listen":
		return "bind a TCP listener at a host-side bind allowlist"
	default:
		return "unknown capability"
	}
}
