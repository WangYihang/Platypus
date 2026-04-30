//go:build linux

package security

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
)

func init() {
	Register(&accountSafetyCheck{})
}

// accountSafetyCheck runs three high-signal passwd/shadow audits
// that show up at the top of every CIS / lynis report:
//
//   1. /etc/passwd, /etc/shadow, /etc/group, /etc/gshadow ownership
//      and mode bits — these files are sensitive enough that a wrong
//      mode is a hardening regression on its own.
//   2. UID 0 duplicates — any account whose UID is 0 except `root`.
//      Almost always either an attacker backdoor or a long-forgotten
//      service account from a misguided shortcut.
//   3. Empty password fields in /etc/shadow — accounts that can be
//      logged into without a password. Usually a sign of a half-
//      finished provisioning script.
//
// Some of these need root to read /etc/shadow. The agent typically
// runs as root; if it can't read shadow we surface that as a check-
// level error rather than silently passing.
type accountSafetyCheck struct{}

func (accountSafetyCheck) ID() string                       { return "accounts.safety" }
func (accountSafetyCheck) Category() string                 { return "accounts" }
func (accountSafetyCheck) Applicable(_ context.Context) bool { return fileExists("/etc/passwd") }
func (accountSafetyCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "Account database integrity (passwd / shadow / UID 0 / empty passwords)",
		Description: "Three audits in one check: file ownership and mode of " +
			"/etc/passwd, /etc/shadow, /etc/group, /etc/gshadow; duplicate UID 0 " +
			"accounts (i.e. any non-root account with effective root); and shadow " +
			"entries with an empty password field (accounts loginable without a " +
			"credential). Reading /etc/shadow requires root or CAP_DAC_READ_SEARCH.",
		References: []string{"CIS 6.1.2-6.1.7", "CIS 6.2.1", "CIS 6.2.2"},
	}
}

func (accountSafetyCheck) Run(_ context.Context) ([]Finding, error) {
	var out []Finding
	out = append(out, fileModeFindings()...)
	out = append(out, uid0DuplicateFindings()...)
	out = append(out, emptyPasswordFindings()...)
	return out, nil
}

type permSpec struct {
	path     string
	maxMode  os.FileMode // any bit set above this is a finding
	wantUID  int
	wantGID  int    // -1 = don't check
	severity string
}

// permSpecs encodes the CIS guidance. /etc/shadow can be 0640 with
// group `shadow` (Debian/Ubuntu) or 0000 (RHEL); accept either.
var permSpecs = []permSpec{
	{"/etc/passwd", 0o644, 0, -1, SeverityMedium},
	{"/etc/group", 0o644, 0, -1, SeverityMedium},
	{"/etc/shadow", 0o640, 0, -1, SeverityHigh},
	{"/etc/gshadow", 0o640, 0, -1, SeverityHigh},
}

func fileModeFindings() []Finding {
	var out []Finding
	for _, spec := range permSpecs {
		st, err := os.Stat(spec.path)
		if err != nil {
			continue
		}
		mode := st.Mode().Perm()
		if mode&^spec.maxMode != 0 {
			out = append(out, Finding{
				ID:          "accounts.perms." + strings.TrimPrefix(spec.path, "/etc/"),
				Category:    "accounts",
				Severity:    spec.severity,
				Title:       spec.path + " has overly permissive mode",
				Description: spec.path + " is sensitive enough that any bit set above the recommended maximum is a hardening regression. Tighten to the conventional value for your distro.",
				Evidence:    fmt.Sprintf("%s mode=%04o (max recommended %04o)", spec.path, mode, spec.maxMode),
				Remediation: fmt.Sprintf("chmod %04o %s; chown root:%s %s", spec.maxMode, spec.path, defaultGroupForFile(spec.path), spec.path),
			})
		}
		if sysStat, ok := st.Sys().(*syscall.Stat_t); ok {
			if int(sysStat.Uid) != spec.wantUID {
				out = append(out, Finding{
					ID:          "accounts.owner." + strings.TrimPrefix(spec.path, "/etc/"),
					Category:    "accounts",
					Severity:    spec.severity,
					Title:       spec.path + " owned by non-root",
					Description: spec.path + " must be owned by uid 0 — any other owner could rewrite the file and pivot to root.",
					Evidence:    fmt.Sprintf("%s uid=%d (want 0)", spec.path, sysStat.Uid),
					Remediation: "chown root " + spec.path,
				})
			}
		}
	}
	return out
}

func defaultGroupForFile(path string) string {
	switch path {
	case "/etc/shadow", "/etc/gshadow":
		return "shadow"
	default:
		return "root"
	}
}

// uid0DuplicateFindings flags any non-root account whose UID is 0.
// These are catastrophic: either the previous admin took a shortcut
// they shouldn't have, or someone planted a backdoor.
func uid0DuplicateFindings() []Finding {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var out []Finding
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) < 3 {
			continue
		}
		name, uid := fields[0], fields[2]
		if uid != "0" || name == "root" {
			continue
		}
		out = append(out, Finding{
			ID:          "accounts.uid0." + sanitiseAccountID(name),
			Category:    "accounts",
			Severity:    SeverityCritical,
			Title:       "Non-root account with UID 0: " + name,
			Description: "Account `" + name + "` has UID 0, giving it the same authority as root. Two near-universal causes: a manual /etc/passwd edit that bypassed `useradd -u 0` checks, or an attacker-installed backdoor account. Either way this is a critical finding — nothing legitimate ships an extra UID 0 account.",
			Evidence:    "passwd entry for " + name + " has uid=0",
			Remediation: "Investigate the origin of this account immediately. If it isn't expected, lock it (`passwd -l " + name + "`), disable its shell, then plan a removal. If it IS expected, confirm with whoever set it up that there isn't a sudoers / capabilities equivalent that would have been safer.",
		})
	}
	return out
}

// emptyPasswordFindings reports shadow entries where the second
// (password) field is empty — login without credential.
func emptyPasswordFindings() []Finding {
	f, err := os.Open("/etc/shadow")
	if err != nil {
		// Most likely: not running as root. Don't surface as a
		// finding (that'd be alarmist) but DO surface as a check-
		// level note via the finding stream so the operator
		// knows we couldn't audit shadow.
		return []Finding{{
			ID:          "accounts.shadow_unreadable",
			Category:    "accounts",
			Severity:    SeverityInfo,
			Title:       "/etc/shadow not readable by the agent",
			Description: "The agent could not open /etc/shadow, which means the empty-password and password-policy audits did not run. Most commonly: the agent is running as a non-root user. Either run the agent as root or grant CAP_DAC_READ_SEARCH.",
			Evidence:    "open /etc/shadow: " + err.Error(),
			Remediation: "Run the agent under a uid with read access to /etc/shadow, or skip this check if your environment forbids it.",
		}}
	}
	defer func() { _ = f.Close() }()

	var out []Finding
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) < 2 {
			continue
		}
		name, hash := fields[0], fields[1]
		if hash != "" {
			continue
		}
		out = append(out, Finding{
			ID:          "accounts.empty_password." + sanitiseAccountID(name),
			Category:    "accounts",
			Severity:    SeverityCritical,
			Title:       "Account `" + name + "` has an empty password",
			Description: "Account `" + name + "` is loginable with an empty password — its shadow entry has no hash. This bypasses every password policy on the host. Almost always a half-finished provisioning script.",
			Evidence:    "shadow entry for " + name + " has empty password field",
			Remediation: "Lock the account immediately: `passwd -l " + name + "` (then audit logins recently as `" + name + "` via /var/log/auth.log or journalctl).",
		})
	}
	return out
}

// sanitiseAccountID strips characters that don't play well with a
// dotted finding id namespace. POSIX usernames technically allow
// dashes, dots, etc.; we drop everything but [A-Za-z0-9_].
func sanitiseAccountID(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "anon"
	}
	return out
}
