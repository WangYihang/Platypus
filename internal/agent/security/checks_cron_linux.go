//go:build linux

package security

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
)

func init() {
	Register(&cronPermsCheck{})
}

// cronPermsCheck verifies the standard cron files and directories
// carry the strict ownership and mode bits CIS prescribes — these
// folders contain code that runs as root, so any group/other write
// turns into a privilege escalation.
type cronPermsCheck struct{}

func (cronPermsCheck) ID() string       { return "cron.permissions" }
func (cronPermsCheck) Category() string { return "cron" }
func (cronPermsCheck) Applicable(_ context.Context) bool {
	return fileExists("/etc/crontab") || dirExists("/etc/cron.d")
}
func (cronPermsCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "Cron file & directory permissions",
		Description: "Inspects /etc/crontab plus /etc/cron.{d,hourly,daily,weekly,monthly} " +
			"for ownership and mode. These files run code as root, so any group/other " +
			"write turns into a privilege escalation. Recommends 0600 on the file and " +
			"0700 on the directory, all owned by uid 0 / gid 0.",
		References: []string{"CIS 5.1.1-5.1.7"},
	}
}

type cronTarget struct {
	path     string
	maxMode  os.FileMode
	severity string
	isDir    bool
}

var cronTargets = []cronTarget{
	{"/etc/crontab", 0o600, SeverityMedium, false},
	{"/etc/cron.hourly", 0o700, SeverityMedium, true},
	{"/etc/cron.daily", 0o700, SeverityMedium, true},
	{"/etc/cron.weekly", 0o700, SeverityMedium, true},
	{"/etc/cron.monthly", 0o700, SeverityMedium, true},
	{"/etc/cron.d", 0o700, SeverityMedium, true},
}

func (cronPermsCheck) Run(_ context.Context) ([]Finding, error) {
	var out []Finding
	for _, t := range cronTargets {
		st, err := os.Stat(t.path)
		if err != nil {
			continue // not present on this distro
		}
		if t.isDir != st.IsDir() {
			continue // shape changed; let the rest of the table proceed
		}
		mode := st.Mode().Perm()
		if mode&^t.maxMode != 0 {
			out = append(out, Finding{
				ID:          "cron.perms." + cronID(t.path),
				Category:    "cron",
				Severity:    t.severity,
				Title:       t.path + " has overly permissive mode",
				Description: t.path + " runs scheduled commands as root. Any bit set above " + fmt.Sprintf("%04o", t.maxMode) + " gives a non-root user a foothold to add or modify cron entries (immediate root code execution next tick).",
				Evidence:    fmt.Sprintf("%s mode=%04o (max recommended %04o)", t.path, mode, t.maxMode),
				Remediation: fmt.Sprintf("chmod %04o %s", t.maxMode, t.path),
			})
		}
		if sysStat, ok := st.Sys().(*syscall.Stat_t); ok {
			if sysStat.Uid != 0 {
				out = append(out, Finding{
					ID:          "cron.owner." + cronID(t.path),
					Category:    "cron",
					Severity:    t.severity,
					Title:       t.path + " not owned by root",
					Description: "Cron tasks run as root; the schedule must be writable only by root.",
					Evidence:    fmt.Sprintf("%s uid=%d (want 0)", t.path, sysStat.Uid),
					Remediation: "chown root:root " + t.path,
				})
			}
		}
	}
	return out, nil
}

func cronID(p string) string {
	return strings.ReplaceAll(strings.TrimPrefix(p, "/etc/"), "/", "_")
}
