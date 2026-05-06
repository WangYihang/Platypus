// sys-security-go is the TinyGo port of example/plugins/system/sys-security.
// Same v2 check set (kernel.version + ssh.config), same wire output
// (protojson SecurityScanResponse / ListSecurityChecksResponse),
// same per-check architecture so adding a check is a single struct
// literal in the registry.
//
// Build: tinygo build -target wasi -o sys_security.wasm .
package main

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

// ---- response shapes (mirrors v2pb encodings) -------------------

type SecurityFinding struct {
	ID          string   `json:"id"`
	CheckID     string   `json:"checkId"`
	Category    string   `json:"category"`
	Severity    string   `json:"severity"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Evidence    string   `json:"evidence"`
	Remediation string   `json:"remediation"`
	References  []string `json:"references,omitempty"`
}

type CheckResult struct {
	ID           string `json:"id"`
	Category     string `json:"category"`
	Status       string `json:"status"` // "ok" | "skipped" | "error"
	Error        string `json:"error,omitempty"`
	ElapsedMs    uint64 `json:"elapsedMs,omitempty"`
	FindingCount uint32 `json:"findingCount,omitempty"`
}

type ScanResponse struct {
	Findings      []SecurityFinding `json:"findings"`
	Checks        []CheckResult     `json:"checks"`
	StartedAtUnix int64             `json:"startedAtUnix,omitempty"`
	ElapsedMs     uint64            `json:"elapsedMs,omitempty"`
	Error         string            `json:"error,omitempty"`
}

type AvailableCheck struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Applicable  bool     `json:"applicable"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	References  []string `json:"references,omitempty"`
}

type ListResponse struct {
	Checks []AvailableCheck `json:"checks"`
	Error  string           `json:"error,omitempty"`
}

// ScanRequest accepts both `check_ids` (snake) and `checkIds`
// (camel) keys — operators may hand-craft requests via the REST API
// in either form, mirroring the Rust crate's serde alias setup.
type ScanRequest struct {
	CheckIDs       []string `json:"checkIds"`
	CheckIDsSnake  []string `json:"check_ids"`
	Categories    []string `json:"categories"`
}

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
			id:          "ssh.config",
			category:    "ssh",
			title:       "SSH server config posture",
			description: "Reads /etc/ssh/sshd_config and flags risky settings: root login over SSH (PermitRootLogin yes) + password authentication (PasswordAuthentication yes — keys-only is the recommended posture).",
			run:         checkSSHConfig,
		},
	}
}

// ---- entry points -----------------------------------------------

//export list_security_checks
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

//export security_scan
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

// ---- check implementations --------------------------------------

func checkKernelVersion() ([]SecurityFinding, bool) {
	raw, err := platypus.HostFSReadString("/proc/sys/kernel/osrelease")
	if err != nil {
		return nil, false // not on linux / not allowed
	}
	return kernelFindings(strings.TrimSpace(raw)), true
}

// kernelFindings is the pure decision layer behind checkKernelVersion.
// Parses an osrelease string, decides whether it predates the 5.10
// LTS line, emits a SecurityFinding when it does. Pure so a host
// build can unit-test it without host_fs_read.
func kernelFindings(osrelease string) []SecurityFinding {
	major, minor := parseKernelMajorMinor(osrelease)
	if major > 5 || (major == 5 && minor >= 10) {
		return nil
	}
	return []SecurityFinding{{
		ID:          "kernel.version.outdated",
		CheckID:     "kernel.version",
		Category:    "kernel",
		Severity:    "medium",
		Title:       "Kernel " + osrelease + " is older than 5.10",
		Description: "Long-term-support kernel lines start at 5.10 (Mar 2021). Hosts on older kernels miss several years of CVE fixes.",
		Evidence:    "/proc/sys/kernel/osrelease = " + osrelease,
		Remediation: "Upgrade to a distribution release that ships a 5.10+ kernel; reboot.",
	}}
}

func parseKernelMajorMinor(s string) (uint32, uint32) {
	parts := strings.SplitN(s, ".", 3)
	var major, minor uint32
	if len(parts) >= 1 {
		if v, err := strconv.ParseUint(parts[0], 10, 32); err == nil {
			major = uint32(v)
		}
	}
	if len(parts) >= 2 {
		// Trim non-numeric suffix from minor: "10-rc4" → "10".
		minorRaw := parts[1]
		end := 0
		for end < len(minorRaw) && minorRaw[end] >= '0' && minorRaw[end] <= '9' {
			end++
		}
		if v, err := strconv.ParseUint(minorRaw[:end], 10, 32); err == nil {
			minor = uint32(v)
		}
	}
	return major, minor
}

func checkSSHConfig() ([]SecurityFinding, bool) {
	raw, err := platypus.HostFSReadString("/etc/ssh/sshd_config")
	if err != nil {
		return nil, false // sshd not installed / unreadable
	}
	return sshdConfigFindings(raw), true
}

// sshdConfigFindings walks an sshd_config-format string, strips
// comments + whitespace, emits a SecurityFinding for each risky
// directive (today: PermitRootLogin yes, PasswordAuthentication yes).
func sshdConfigFindings(raw string) []SecurityFinding {
	var findings []SecurityFinding
	for _, line := range strings.Split(raw, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch {
		case fields[0] == "PermitRootLogin" && fields[1] == "yes":
			findings = append(findings, SecurityFinding{
				ID:          "ssh.permit_root_login",
				CheckID:     "ssh.config",
				Category:    "ssh",
				Severity:    "high",
				Title:       "SSH server allows root login",
				Description: "PermitRootLogin yes lets anyone with the root password (or root SSH key) authenticate as root directly. Best practice: log in as a non-root user + use sudo.",
				Evidence:    "/etc/ssh/sshd_config: PermitRootLogin yes",
				Remediation: `Set PermitRootLogin to "no" (or "prohibit-password") in /etc/ssh/sshd_config and restart sshd.`,
			})
		case fields[0] == "PasswordAuthentication" && fields[1] == "yes":
			findings = append(findings, SecurityFinding{
				ID:          "ssh.password_authentication",
				CheckID:     "ssh.config",
				Category:    "ssh",
				Severity:    "medium",
				Title:       "SSH server allows password authentication",
				Description: "Password auth is brute-forceable. Public-key authentication has no such failure mode and is the recommended posture for production hosts.",
				Evidence:    "/etc/ssh/sshd_config: PasswordAuthentication yes",
				Remediation: `Distribute SSH keys to users, set PasswordAuthentication to "no" in /etc/ssh/sshd_config, restart sshd.`,
			})
		}
	}
	return findings
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
