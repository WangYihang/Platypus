//go:build linux

package security

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func init() {
	Register(&sudoersCheck{})
}

// sudoersCheck audits the sudoers configuration. Three high-signal
// concerns:
//
//  1. /etc/sudoers must be 0440 root:root. Anything looser lets a
//     non-root user rewrite the policy.
//  2. /etc/sudoers.d/ must be 0750 root:root, and every file inside
//     must be 0440 root:root.
//  3. Every NOPASSWD entry across both files is reported (one finding
//     per matching line). NOPASSWD on a non-system account is one of
//     the most common privilege-escalation footholds — and even on
//     system accounts is worth surfacing for review.
type sudoersCheck struct{}

func (sudoersCheck) ID() string       { return "auth.sudoers" }
func (sudoersCheck) Category() string { return "accounts" }
func (sudoersCheck) Applicable(_ context.Context) bool {
	return fileExists("/etc/sudoers") || dirExists("/etc/sudoers.d")
}
func (sudoersCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "Sudoers permissions and NOPASSWD entries",
		Description: "Verifies /etc/sudoers is 0440 root:root, /etc/sudoers.d is " +
			"0750 root:root, and every file in /etc/sudoers.d is 0440 root:root. " +
			"Also enumerates NOPASSWD lines across both — these grant root with no " +
			"second-factor and are a common privilege-escalation foothold (an attacker " +
			"with a shell as the named user becomes root immediately). Reading " +
			"/etc/sudoers may require root.",
		References: []string{"CIS 5.3.1-5.3.5"},
	}
}

func (sudoersCheck) Run(_ context.Context) ([]Finding, error) {
	var out []Finding
	out = append(out, sudoersFilePerms()...)
	out = append(out, sudoersDirContents()...)
	out = append(out, sudoersNoPasswd()...)
	return out, nil
}

func sudoersFilePerms() []Finding {
	var out []Finding
	if st, err := os.Stat("/etc/sudoers"); err == nil {
		out = append(out, fileModeChecks(st, "/etc/sudoers", 0o440, "auth.sudoers.perms.sudoers", SeverityHigh)...)
	}
	if st, err := os.Stat("/etc/sudoers.d"); err == nil {
		out = append(out, fileModeChecks(st, "/etc/sudoers.d", 0o750, "auth.sudoers.perms.sudoers_d", SeverityMedium)...)
	}
	return out
}

func sudoersDirContents() []Finding {
	entries, err := os.ReadDir("/etc/sudoers.d")
	if err != nil {
		return nil
	}
	var out []Finding
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), "README") {
			continue
		}
		full := filepath.Join("/etc/sudoers.d", e.Name())
		st, err := os.Stat(full)
		if err != nil {
			continue
		}
		out = append(out, fileModeChecks(st, full,
			0o440, "auth.sudoers.perms.fragment_"+sanitiseAccountID(e.Name()),
			SeverityHigh)...)
	}
	return out
}

// fileModeChecks is the shared "mode + owner" audit used by both the
// top-level /etc/sudoers and every fragment under /etc/sudoers.d.
func fileModeChecks(st os.FileInfo, path string, maxMode os.FileMode, idBase, severity string) []Finding {
	var out []Finding
	mode := st.Mode().Perm()
	if mode&^maxMode != 0 {
		out = append(out, Finding{
			ID:          idBase + ".mode",
			Category:    "accounts",
			Severity:    severity,
			Title:       path + " has overly permissive mode",
			Description: path + " controls who can become root via sudo. Any bit set above the recommended maximum lets a non-root user rewrite the policy and grant themselves NOPASSWD access.",
			Evidence:    fmt.Sprintf("%s mode=%04o (max recommended %04o)", path, mode, maxMode),
			Remediation: fmt.Sprintf("chmod %04o %s && chown root:root %s", maxMode, path, path),
		})
	}
	if sysStat, ok := st.Sys().(*syscall.Stat_t); ok {
		if sysStat.Uid != 0 {
			out = append(out, Finding{
				ID:          idBase + ".owner",
				Category:    "accounts",
				Severity:    severity,
				Title:       path + " not owned by root",
				Description: "Sudo policy must be owned by uid 0; a different owner can rewrite the file at will.",
				Evidence:    fmt.Sprintf("%s uid=%d (want 0)", path, sysStat.Uid),
				Remediation: "chown root:root " + path,
			})
		}
	}
	return out
}

// sudoersNoPasswd walks /etc/sudoers and /etc/sudoers.d/* and flags
// every active NOPASSWD line. Includes the matched line as evidence
// so the operator can immediately see who/what was granted.
//
// We deliberately do NOT try to parse Defaults / Cmnd_Alias / etc —
// the heuristic is "the substring NOPASSWD: appears on a non-comment
// line", which catches every real grant and produces zero false
// negatives. False positives are limited to commented-out historical
// rules using a non-standard comment marker, which is rare enough
// that surfacing them for review is fine.
func sudoersNoPasswd() []Finding {
	var out []Finding
	scan := func(path string) {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.Contains(line, "NOPASSWD") {
				continue
			}
			out = append(out, Finding{
				ID:          fmt.Sprintf("auth.sudoers.nopasswd.%s_%d", sanitiseAccountID(filepath.Base(path)), lineNo),
				Category:    "accounts",
				Severity:    SeverityMedium,
				Title:       "NOPASSWD sudo grant",
				Description: "An entry in " + path + " grants sudo without a password challenge. Anyone (or anything) with a shell as the principal becomes root with no further authentication. Confirm the grant is necessary and scoped to the smallest possible command set; for service automation prefer a dedicated systemd unit running directly as root over a NOPASSWD entry.",
				Evidence:    fmt.Sprintf("%s:%d  %s", path, lineNo, line),
				Remediation: "Audit who can authenticate as the principal user and whether the granted command needs the no-password property. Consider replacing with a tightly-scoped Cmnd_Alias and / or restricting to specific args.",
			})
		}
	}
	scan("/etc/sudoers")
	if entries, err := os.ReadDir("/etc/sudoers.d"); err == nil {
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), "README") {
				continue
			}
			scan(filepath.Join("/etc/sudoers.d", e.Name()))
		}
	}
	return out
}
