//go:build wasip1

// sys-security-go is the TinyGo / wasip1 port of
// example/plugins/system/sys-security. Same v3 check set, same wire
// output (protojson SecurityScanResponse / ListSecurityChecksResponse),
// same per-check architecture so adding a check is a single struct
// literal in registry().
//
// Coverage:
//   - kernel.version       /proc/sys/kernel/osrelease vs 5.10 LTS floor
//   - kernel.mitigations   /sys/devices/system/cpu/vulnerabilities/*
//   - ssh.config           /etc/ssh/sshd_config risky directives
//   - sysctl.posture       10 hardening sysctls
//   - fs.path_writable     world-writable PATH dirs
//   - fs.suid_outliers     setuid/setgid binaries off the allowlist
//
// Build: tinygo build -target wasi -o sys_security.wasm .
//
// Note: the build constraint is wasip1-only. The SDK (extism + the
// platypus host bindings) emits //go:wasmimport directives that don't
// compile under stock host GOOS. The pure decision-layer logic lives
// in pure.go (no build tag) so `go test ./...` exercises it.
package main

import (
	"encoding/json"
	"strings"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

// ---- entry points -----------------------------------------------
//
// //go:wasmexport is honored by both stock Go (1.21+) and TinyGo
// 0.31+. Older `//export` only worked under TinyGo, which is why the
// new directive is preferred here.

//go:wasmexport list_security_checks
func listSecurityChecks() int32 {
	checks := registry()
	out := ListResponse{Checks: make([]AvailableCheck, 0, len(checks))}
	for _, c := range checks {
		out.Checks = append(out.Checks, AvailableCheck{
			ID:          c.id,
			Category:    c.category,
			Applicable:  true,
			Title:       c.title,
			Description: c.description,
		})
	}
	body, err := json.Marshal(out)
	if err != nil {
		platypus.LogErrorf("sys-security-go: marshal: %s", err.Error())
		return 1
	}
	pdk.OutputString(string(body))
	return 0
}

//go:wasmexport security_scan
func securityScan() int32 {
	var req ScanRequest
	if input := pdk.Input(); len(input) > 0 {
		_ = json.Unmarshal(input, &req)
	}
	want := append([]string(nil), req.CheckIDs...)
	want = append(want, req.CheckIDsSnake...)

	resp := ScanResponse{
		Findings: make([]SecurityFinding, 0),
		Checks:   make([]CheckResult, 0),
	}
	for _, c := range registry() {
		if len(want) > 0 && !contains(want, c.id) {
			continue
		}
		if len(req.Categories) > 0 && !contains(req.Categories, c.category) {
			continue
		}
		fs, applicable := c.run()
		status := "ok"
		if !applicable {
			status = "skipped"
		}
		resp.Checks = append(resp.Checks, CheckResult{
			ID:           c.id,
			Category:     c.category,
			Status:       status,
			FindingCount: uint32(len(fs)),
		})
		resp.Findings = append(resp.Findings, fs...)
	}
	body, err := json.Marshal(resp)
	if err != nil {
		platypus.LogErrorf("sys-security-go: marshal: %s", err.Error())
		return 1
	}
	pdk.OutputString(string(body))
	return 0
}

func main() {}

// ---- registered checks ------------------------------------------

type check struct {
	id, category, title, description string
	run                              func() ([]SecurityFinding, bool)
}

func registry() []check {
	return []check{
		{
			id:          "kernel.version",
			category:    "kernel",
			title:       "Kernel version is recent",
			description: "Hosts on a kernel older than 5.10 are missing several years of CVE fixes; long-term-support lines start at 5.10 (Mar 2021). This check parses /proc/sys/kernel/osrelease.",
			run:         checkKernelVersion,
		},
		{
			id:          "kernel.mitigations",
			category:    "kernel",
			title:       "CPU vulnerability mitigations active",
			description: "Reads each file under /sys/devices/system/cpu/vulnerabilities/ and flags any whose first token is not 'Mitigation:' or 'Not affected'. Catches Spectre/Meltdown/MDS/L1TF/etc. running with mitigations disabled (often via mitigations=off boot flag).",
			run:         checkKernelMitigations,
		},
		{
			id:          "ssh.config",
			category:    "ssh",
			title:       "SSH server config posture",
			description: "Reads /etc/ssh/sshd_config and flags risky settings: PermitRootLogin yes, PasswordAuthentication yes, PermitEmptyPasswords yes, X11Forwarding yes.",
			run:         checkSSHConfig,
		},
		{
			id:          "sysctl.posture",
			category:    "sysctl",
			title:       "Sysctl hardening posture",
			description: "Reads a curated set of /proc/sys keys (kptr_restrict, dmesg_restrict, unprivileged_bpf_disabled, rp_filter, accept_redirects, send_redirects, tcp_syncookies, fs.protected_hardlinks/symlinks, fs.suid_dumpable). One finding per misaligned key.",
			run:         checkSysctlPosture,
		},
		{
			id:          "fs.path_writable",
			category:    "filesystem",
			title:       "World-writable directories on PATH",
			description: "Stats each PATH directory (plus the standard fallback set /usr/local/sbin, /usr/local/bin, /usr/sbin, /usr/bin, /sbin, /bin, /snap/bin) and flags any that are world-writable AND non-sticky — the textbook setup for an unprivileged user to swap out a binary that root will later invoke.",
			run:         checkFSPathWritable,
		},
		{
			id:          "fs.suid_outliers",
			category:    "filesystem",
			title:       "Unexpected setuid/setgid binaries",
			description: "Lists /usr/bin, /usr/sbin, /usr/local/bin, /usr/local/sbin, /bin, /sbin, /opt and flags setuid/setgid binaries not on the allowlist of well-known distro helpers. Capped at 20,000 visited entries.",
			run:         checkFSSuidOutliers,
		},
	}
}

// ---- check runners (host-side I/O) ------------------------------

func checkKernelVersion() ([]SecurityFinding, bool) {
	raw, err := platypus.HostFSReadString("/proc/sys/kernel/osrelease")
	if err != nil {
		return nil, false // not on linux / not allowed
	}
	return kernelFindings(strings.TrimSpace(raw)), true
}

func checkKernelMitigations() ([]SecurityFinding, bool) {
	const root = "/sys/devices/system/cpu/vulnerabilities"
	entries, err := platypus.HostFSListDir(root)
	if err != nil {
		return nil, false // /sys not exposed (containers)
	}
	var out []SecurityFinding
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		body, err := platypus.HostFSReadString(root + "/" + e.Name)
		if err != nil {
			continue
		}
		out = append(out, mitigationFindings(e.Name, body)...)
	}
	return out, true
}

func checkSSHConfig() ([]SecurityFinding, bool) {
	raw, err := platypus.HostFSReadString("/etc/ssh/sshd_config")
	if err != nil {
		return nil, false // sshd not installed / unreadable
	}
	return sshdConfigFindings(raw), true
}

func checkSysctlPosture() ([]SecurityFinding, bool) {
	// Cheap applicability probe: if /proc/sys is not exposed (e.g.
	// restricted container), skip the whole check.
	if _, err := platypus.HostFSReadString("/proc/sys/kernel/osrelease"); err != nil {
		return nil, false
	}
	var findings []SecurityFinding
	for _, e := range sysctlExpectations() {
		raw, err := platypus.HostFSReadString(sysctlPath(e.key))
		if err != nil {
			continue // missing keys are common on minimal containers
		}
		findings = append(findings, sysctlFinding(e, normalizeSysctl(raw))...)
	}
	return findings, true
}

func checkFSPathWritable() ([]SecurityFinding, bool) {
	environ, _ := platypus.HostFSReadString("/proc/1/environ")
	paths := parsePathEnv(environ)
	for _, p := range pathFallback {
		if !contains(paths, p) {
			paths = append(paths, p)
		}
	}

	var findings []SecurityFinding
	for _, d := range paths {
		entry, ok := statViaParent(d)
		if !ok {
			continue
		}
		if !entry.IsDir {
			continue
		}
		if entry.Mode&0o002 == 0 {
			continue
		}
		// World-writable + sticky (1777) is the /tmp pattern and is
		// intentional; flag only the non-sticky case.
		if entry.Mode&0o1000 != 0 {
			continue
		}
		findings = append(findings, pathWritableFinding(d, entry.Mode))
	}
	return findings, true
}

// statViaParent fetches a directory's listdir entry from its parent —
// gets us mode bits without a separate stat host fn.
func statViaParent(path string) (platypus.FSListEntry, bool) {
	parent, name, ok := splitParent(path)
	if !ok {
		return platypus.FSListEntry{}, false
	}
	entries, err := platypus.HostFSListDir(parent)
	if err != nil {
		return platypus.FSListEntry{}, false
	}
	for _, e := range entries {
		if e.Name == name {
			return e, true
		}
	}
	return platypus.FSListEntry{}, false
}

func checkFSSuidOutliers() ([]SecurityFinding, bool) {
	var findings []SecurityFinding
	visited := 0

	for _, root := range suidScanRoots {
		stack := []string{root}
		for len(stack) > 0 {
			if visited >= suidScanCap {
				return findings, true
			}
			dir := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			entries, err := platypus.HostFSListDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				visited++
				if visited >= suidScanCap {
					return findings, true
				}
				path := dir + "/" + e.Name
				if e.IsDir {
					if depthBelow(root, path) < 4 {
						stack = append(stack, path)
					}
					continue
				}
				if f, ok := suidOutlierFinding(path, e.Name, e.Mode); ok {
					findings = append(findings, f)
				}
			}
		}
	}
	return findings, true
}
