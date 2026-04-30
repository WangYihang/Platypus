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
	Register(&loginDefsCheck{})
}

// loginDefsCheck pulls a few high-impact knobs out of /etc/login.defs
// and reports any that diverge from the recommended posture: max
// password age, min password age, warning age, and the password
// hashing algorithm. CIS 5.5.1 territory.
//
// Skipping intentionally:
//
//	· LOGIN_RETRIES — most distros override via PAM, where the truth
//	  lives in /etc/security/faillock.conf or similar; auditing both
//	  sources is its own checker.
//	· UMASK — not directly a security finding (and the spec values
//	  vary by use case).
type loginDefsCheck struct {
	path string
}

func (c *loginDefsCheck) ID() string                        { return "auth.login_defs" }
func (c *loginDefsCheck) Category() string                  { return "accounts" }
func (c *loginDefsCheck) Applicable(_ context.Context) bool { return fileExists(c.pathOrDefault()) }
func (c *loginDefsCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "/etc/login.defs password policy",
		Description: "Parses /etc/login.defs for PASS_MAX_DAYS, PASS_MIN_DAYS, " +
			"PASS_WARN_AGE, and ENCRYPT_METHOD. These set the system-wide default " +
			"password ageing and hashing choices. Per-user overrides in chage and " +
			"PAM-supplied policies are NOT inspected here — `chage -l <user>` and " +
			"/etc/pam.d/common-password remain the source of truth for individual " +
			"accounts.",
		References: []string{"CIS 5.5.1"},
	}
}

func (c *loginDefsCheck) pathOrDefault() string {
	if c.path != "" {
		return c.path
	}
	return "/etc/login.defs"
}

func (c *loginDefsCheck) Run(_ context.Context) ([]Finding, error) {
	values, err := parseLoginDefs(c.pathOrDefault())
	if err != nil {
		return nil, err
	}
	var out []Finding

	if maxDays, ok := intValue(values, "PASS_MAX_DAYS"); ok {
		if maxDays > 365 || maxDays <= 0 {
			out = append(out, Finding{
				ID: "auth.login_defs.pass_max_days", Category: "accounts",
				Severity: SeverityLow,
				Title:    "PASS_MAX_DAYS too long or disabled",
				Description: "Passwords without a forced rotation policy stay valid forever. " +
					"CIS recommends a max of 365 days; many sites prefer 90.",
				Evidence:    fmt.Sprintf("PASS_MAX_DAYS=%d", maxDays),
				Remediation: "Set PASS_MAX_DAYS to 365 (or shorter) in /etc/login.defs and run `chage --maxdays 365 <user>` to apply to existing accounts.",
			})
		}
	}
	if minDays, ok := intValue(values, "PASS_MIN_DAYS"); ok {
		if minDays < 1 {
			out = append(out, Finding{
				ID: "auth.login_defs.pass_min_days", Category: "accounts",
				Severity:    SeverityLow,
				Title:       "PASS_MIN_DAYS allows immediate password rotation",
				Description: "Without a minimum age, users (or attackers with a temporary credential) can churn through passwords to escape a history check.",
				Evidence:    fmt.Sprintf("PASS_MIN_DAYS=%d", minDays),
				Remediation: "Set PASS_MIN_DAYS to at least 1.",
			})
		}
	}
	if warn, ok := intValue(values, "PASS_WARN_AGE"); ok {
		if warn < 7 {
			out = append(out, Finding{
				ID: "auth.login_defs.pass_warn_age", Category: "accounts",
				Severity:    SeverityInfo,
				Title:       "PASS_WARN_AGE shorter than the CIS recommendation",
				Description: "Users should get at least a week of advance warning before their password expires.",
				Evidence:    fmt.Sprintf("PASS_WARN_AGE=%d", warn),
				Remediation: "Set PASS_WARN_AGE to 7 or more in /etc/login.defs.",
			})
		}
	}
	if method, ok := values["ENCRYPT_METHOD"]; ok {
		method = strings.ToUpper(method)
		switch method {
		case "SHA512", "YESCRYPT":
			// fine
		default:
			out = append(out, Finding{
				ID: "auth.login_defs.encrypt_method", Category: "accounts",
				Severity:    SeverityHigh,
				Title:       "ENCRYPT_METHOD weaker than SHA512 / YESCRYPT",
				Description: "MD5 and DES password hashes are trivially crackable on modern hardware. SHA512 (or YESCRYPT on newer distros) is the baseline recommendation.",
				Evidence:    "ENCRYPT_METHOD=" + method,
				Remediation: "Set ENCRYPT_METHOD to SHA512 (or YESCRYPT) in /etc/login.defs and reset existing passwords so the new hash takes effect.",
			})
		}
	}
	return out, nil
}

func parseLoginDefs(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		out[strings.ToUpper(fields[0])] = fields[1]
	}
	return out, sc.Err()
}

func intValue(m map[string]string, key string) (int, bool) {
	if v, ok := m[key]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n, true
		}
	}
	return 0, false
}
