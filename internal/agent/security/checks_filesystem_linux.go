//go:build linux

package security

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Register(&worldWritablePathCheck{})
	Register(&suidOutlierCheck{})
}

// worldWritablePathCheck inspects every directory on the root user's
// PATH (and a fixed fallback set) for o+w on the directory itself.
// A writable PATH entry lets any local user replace the binaries
// every privileged command resolves through, which is a textbook
// privilege escalation. We deliberately do NOT recurse — recursing
// into /usr/bin would list tens of thousands of files and the
// finding we care about is at the directory level.
type worldWritablePathCheck struct{}

func (worldWritablePathCheck) ID() string                       { return "fs.path_writable" }
func (worldWritablePathCheck) Category() string                 { return "filesystem" }
func (worldWritablePathCheck) Applicable(_ context.Context) bool { return true }

func (worldWritablePathCheck) Run(_ context.Context) ([]Finding, error) {
	dirs := candidatePathDirs()
	var findings []Finding
	for _, d := range dirs {
		st, err := os.Stat(d)
		if err != nil || !st.IsDir() {
			continue
		}
		mode := st.Mode().Perm()
		if mode&0o002 == 0 {
			continue
		}
		// World-writable + sticky (1777) is the /tmp pattern and is
		// intentional; flag only the non-sticky case here.
		if st.Mode()&os.ModeSticky != 0 {
			continue
		}
		findings = append(findings, Finding{
			ID:          "fs.path_writable",
			Category:    "filesystem",
			Severity:    SeverityCritical,
			Title:       "World-writable directory on PATH",
			Description: "Any local user can replace binaries in this directory. The next time root (or any other account) invokes a command that resolves here, it executes attacker-controlled code.",
			Evidence:    fmt.Sprintf("%s mode=%04o", d, mode),
			Remediation: fmt.Sprintf("chmod o-w %s; investigate how the directory got created with this mode (often a packaging bug or a misconfigured deploy script).", d),
		})
	}
	return findings, nil
}

func candidatePathDirs() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		if p == "" {
			return
		}
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	// Live PATH catches per-deployment additions; the static fallback
	// catches environments where PATH was scrubbed (e.g. systemd
	// service unit with a minimal PATH).
	for _, p := range strings.Split(os.Getenv("PATH"), ":") {
		add(p)
	}
	for _, p := range []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin", "/sbin", "/bin", "/snap/bin"} {
		add(p)
	}
	return out
}

// suidOutlierCheck enumerates SUID and SGID binaries under the
// hardcoded "places where Linux distros normally put binaries" list
// and flags anything that isn't on a small allowlist of well-known
// distro binaries. The allowlist is deliberately tight; the goal is
// to surface custom or unexpected SUID files (the classic LPE
// vector) for a human to triage, not to authoritatively classify
// every distro binary as "expected".
//
// Performance: capped at suidScanCap files visited in total so a
// pathological filesystem (chroot loops, an /opt with a million
// files) can't stall the scan.
type suidOutlierCheck struct{}

func (suidOutlierCheck) ID() string                       { return "fs.suid_outliers" }
func (suidOutlierCheck) Category() string                 { return "filesystem" }
func (suidOutlierCheck) Applicable(_ context.Context) bool { return true }

const suidScanCap = 20000

// suidAllowlist is the set of binary base names that legitimately
// ship as setuid/setgid on common Linux distros. Anything else
// surfaces as a finding for an operator to review. The list is
// short on purpose — long allowlists rot fast and the cost of a
// false positive here is a one-line review, not pager noise.
var suidAllowlist = map[string]struct{}{
	"chage": {}, "chfn": {}, "chsh": {}, "crontab": {}, "expiry": {},
	"fusermount": {}, "fusermount3": {}, "gpasswd": {}, "mount": {},
	"newgidmap": {}, "newgrp": {}, "newuidmap": {}, "passwd": {},
	"pkexec": {}, "ping": {}, "ping6": {}, "pmount": {}, "pumount": {},
	"sg": {}, "ssh-agent": {}, "ssh-keysign": {}, "su": {}, "sudo": {},
	"sudoedit": {}, "sudo_logsrvd": {}, "umount": {},
	// systemd / dbus helpers seen on modern systemd distros.
	"polkit-agent-helper-1": {}, "dbus-daemon-launch-helper": {},
	"unix_chkpwd": {}, "Xorg.wrap": {},
	// Wrapper PAM helpers.
	"pam_timestamp_check": {}, "utempter": {}, "write": {}, "wall": {},
	// OpenBSD / Debian doas.
	"doas": {},
}

var suidScanRoots = []string{"/usr/bin", "/usr/sbin", "/usr/local/bin", "/usr/local/sbin", "/bin", "/sbin", "/opt"}

func (suidOutlierCheck) Run(ctx context.Context) ([]Finding, error) {
	var findings []Finding
	visited := 0

	for _, root := range suidScanRoots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					// Permission denied on a subtree: skip the subtree
					// rather than aborting the whole scan.
					return fs.SkipDir
				}
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			visited++
			if visited > suidScanCap {
				return fs.SkipAll
			}
			if d.IsDir() {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				return nil
			}
			mode := info.Mode()
			if mode&os.ModeSetuid == 0 && mode&os.ModeSetgid == 0 {
				return nil
			}
			if _, ok := suidAllowlist[d.Name()]; ok {
				return nil
			}
			bit := "setuid"
			if mode&os.ModeSetgid != 0 && mode&os.ModeSetuid == 0 {
				bit = "setgid"
			}
			findings = append(findings, Finding{
				ID:          "fs.suid_outlier",
				Category:    "filesystem",
				Severity:    SeverityMedium,
				Title:       "Unexpected " + bit + " binary",
				Description: "This binary carries a privileged-execution bit but is not on the agent's allowlist of well-known distro setuid programs. Attacker-installed or vendor-bundled setuid binaries are a common privilege-escalation vector and worth a human review.",
				Evidence:    fmt.Sprintf("%s mode=%v", path, mode.String()),
				Remediation: "Confirm the binary is expected (often it's a SUID helper from a third-party package). If not needed, remove the bit with `chmod u-s` / `chmod g-s` or uninstall the package.",
			})
			return nil
		})
		if err != nil && err != fs.SkipAll {
			// Surface walk-level errors, but don't lose the partial
			// findings — they're still useful.
			return findings, err
		}
	}
	return findings, nil
}
