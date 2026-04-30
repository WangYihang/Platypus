//go:build linux

package security

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func init() {
	Register(&pwqualityCheck{})
}

// pwqualityCheck audits the libpwquality settings that govern the
// strength of new passwords. The actual rules sit in two places:
//
//	/etc/security/pwquality.conf  — preferred since libpwquality 1.4
//	/etc/pam.d/{common-password, system-auth}  — legacy / overrides
//
// We parse pwquality.conf when present (key=value) and fall back to
// scanning the PAM stack for the `pam_pwquality.so` line and
// extracting space-separated arguments. CIS 5.4.1 is the source.
//
// Skipping intentionally: pam_unix retry / remember (separate
// concerns and the PAM stack interaction is hairy), pam_faillock
// lockout policy (also its own checker).
type pwqualityCheck struct{}

func (pwqualityCheck) ID() string                        { return "auth.pwquality" }
func (pwqualityCheck) Category() string                  { return "accounts" }
func (pwqualityCheck) Applicable(_ context.Context) bool { return pwqualityApplicable() }
func (pwqualityCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "PAM password complexity (libpwquality)",
		Description: "Reads /etc/security/pwquality.conf and the PAM password " +
			"stack (/etc/pam.d/common-password on Debian/Ubuntu, /etc/pam.d/system-auth " +
			"on RHEL/Fedora) to verify minlen ≥ 14 and at least one credit class " +
			"(digit / upper / lower / other) is required. PAM stacks are notoriously " +
			"distro-specific; consider this an indicator, not the final word.",
		References: []string{"CIS 5.4.1"},
	}
}

func pwqualityApplicable() bool {
	for _, p := range pwqualityCandidatePaths() {
		if fileExists(p) {
			return true
		}
	}
	return false
}

func pwqualityCandidatePaths() []string {
	return []string{
		"/etc/security/pwquality.conf",
		"/etc/pam.d/common-password", // Debian/Ubuntu
		"/etc/pam.d/system-auth",     // RHEL/Fedora
		"/etc/pam.d/password-auth",   // RHEL alt
	}
}

func (pwqualityCheck) Run(_ context.Context) ([]Finding, error) {
	settings := map[string]string{}
	// Order: pwquality.conf first (authoritative when present), then
	// PAM stack overrides (which OVERRIDE the conf for the matching
	// argument). Last-write-wins matches the runtime semantics of
	// pam_pwquality.so loading the conf and then applying its own
	// CLI args on top.
	if vals, ok := readPwqualityConf("/etc/security/pwquality.conf"); ok {
		for k, v := range vals {
			settings[k] = v
		}
	}
	for _, p := range []string{"/etc/pam.d/common-password", "/etc/pam.d/system-auth", "/etc/pam.d/password-auth"} {
		if vals, ok := readPwqualityFromPAM(p); ok {
			for k, v := range vals {
				settings[k] = v
			}
		}
	}

	var out []Finding

	if minlen, ok := intValue(settings, "minlen"); ok {
		if minlen < 14 {
			out = append(out, Finding{
				ID: "auth.pwquality.minlen", Category: "accounts",
				Severity:    SeverityMedium,
				Title:       "Password minimum length below CIS recommendation",
				Description: "libpwquality enforces minlen at the time a password is set. CIS recommends 14 characters; below that bumps brute-force time on a stolen hash from years to days.",
				Evidence:    fmt.Sprintf("minlen=%d", minlen),
				Remediation: "Set minlen=14 in /etc/security/pwquality.conf and reload PAM.",
			})
		}
	}
	// Credit checks — at least one of dcredit/ucredit/lcredit/ocredit
	// should be < 0 (negative = "require at least N", positive =
	// "credit per N", 0 = neutral). If all four are 0 or positive,
	// the password "Password1" passes — that's the finding.
	creditNegative := false
	for _, key := range []string{"dcredit", "ucredit", "lcredit", "ocredit"} {
		if v, ok := intValue(settings, key); ok && v < 0 {
			creditNegative = true
			break
		}
	}
	// Only fire when we found at least ONE pwquality setting overall
	// — otherwise we'd nag users on minimal containers that have no
	// password policy at all by design.
	if !creditNegative && len(settings) > 0 {
		out = append(out, Finding{
			ID: "auth.pwquality.credits", Category: "accounts",
			Severity:    SeverityLow,
			Title:       "Password complexity credits not enforced",
			Description: "None of dcredit / ucredit / lcredit / ocredit is set negative, so libpwquality won't require any character class. \"Password1\" passes as easily as a 14-character random string.",
			Evidence:    fmt.Sprintf("dcredit=%s ucredit=%s lcredit=%s ocredit=%s", settings["dcredit"], settings["ucredit"], settings["lcredit"], settings["ocredit"]),
			Remediation: "In /etc/security/pwquality.conf, set at least dcredit=-1 and ucredit=-1 (or ocredit=-1).",
		})
	}
	return out, nil
}

// readPwqualityConf parses key=value lines from
// /etc/security/pwquality.conf. Comments (#) and blank lines
// ignored. Returns ok=false when the file isn't present.
func readPwqualityConf(path string) (map[string]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		out[strings.ToLower(k)] = v
	}
	return out, true
}

// readPwqualityFromPAM scans a PAM stack file for the
// pam_pwquality.so line and pulls space-separated `key=value` args
// off it. Lines starting with # are ignored. Lines without
// `pam_pwquality.so` are ignored.
func readPwqualityFromPAM(path string) (map[string]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	out := map[string]string{}
	any := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "pam_pwquality.so") {
			continue
		}
		any = true
		for _, tok := range strings.Fields(line) {
			eq := strings.Index(tok, "=")
			if eq <= 0 {
				continue
			}
			k := strings.ToLower(tok[:eq])
			v := tok[eq+1:]
			// Sanity-skip any tokens that don't look like the
			// arguments we care about (PAM `module-arg` shape).
			if _, err := strconv.Atoi(v); err == nil || k == "minlen" {
				out[k] = v
			}
		}
	}
	if !any {
		return nil, false
	}
	return out, true
}
