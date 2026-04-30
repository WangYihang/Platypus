//go:build linux

package security

import (
	"context"
	"fmt"
	"os"
)

func init() {
	Register(&auditdCheck{})
}

// auditdCheck reports two related conditions:
//
//  1. auditd is not running — auditd-or-equivalent is the canonical
//     Linux audit pipeline (CIS 4.1). Without it, post-incident
//     forensic data is limited to whatever syslog captured.
//  2. /etc/audit/auditd.conf is missing — even when the daemon is
//     not running, the absence of the configuration file means the
//     package isn't installed, which is its own (lower-severity)
//     finding because it implies the audit pipeline can't be
//     brought up without provisioning.
//
// We don't audit the rule set itself — CIS lists ~30 specific rules
// (4.1.3, 4.1.4, …) and the right rules vary by use case. That's a
// future "auditd.rules" checker if there's appetite.
type auditdCheck struct{}

func (auditdCheck) ID() string                        { return "audit.auditd" }
func (auditdCheck) Category() string                  { return "audit" }
func (auditdCheck) Applicable(_ context.Context) bool { return true }
func (auditdCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "auditd presence and runtime status",
		Description: "Reports whether the auditd daemon is running and whether " +
			"/etc/audit/auditd.conf exists. Doesn't audit the rule set itself — CIS " +
			"recommends ~30 specific audit rules that vary by use case (4.1.3-4.1.18); " +
			"those would be a separate, opinionated checker.",
		References: []string{"CIS 4.1.1.1", "CIS 4.1.1.2"},
	}
}

func (auditdCheck) Run(_ context.Context) ([]Finding, error) {
	var out []Finding

	confPresent := fileExists("/etc/audit/auditd.conf")
	running := anyProcessNamed("auditd", "auditdqd")

	if !confPresent {
		out = append(out, Finding{
			ID:          "audit.auditd.not_installed",
			Category:    "audit",
			Severity:    SeverityLow,
			Title:       "auditd not installed",
			Description: "/etc/audit/auditd.conf is absent. The audit subsystem can't be enabled without provisioning the package (auditd on Debian/Ubuntu, audit on RHEL/Fedora). Operators with another audit pipeline (laurel + auditbeat, ebpf-based collectors, …) can ignore this.",
			Evidence:    "/etc/audit/auditd.conf does not exist",
			Remediation: "Install the audit package: `apt install auditd` (Debian/Ubuntu) or `dnf install audit` (RHEL/Fedora), then `systemctl enable --now auditd`.",
		})
		return out, nil
	}
	if !running {
		out = append(out, Finding{
			ID:          "audit.auditd.not_running",
			Category:    "audit",
			Severity:    SeverityMedium,
			Title:       "auditd installed but not running",
			Description: "/etc/audit/auditd.conf exists but no auditd process is alive. Audit events are dropped by the kernel when no listener is attached, so post-incident forensic data is limited to whatever syslog captured.",
			Evidence:    "/etc/audit/auditd.conf present; no auditd process found in /proc",
			Remediation: "Start the daemon: `systemctl enable --now auditd` (or `service auditd start` on sysv hosts).",
		})
	}
	// Bonus: warn if /var/log/audit exists but the daemon isn't
	// rotating it — a 4 GB audit.log eats the disk on long-running
	// hosts. Cheap to add; using os.Stat here keeps the agent
	// portable (no audit-log path API).
	if st, err := os.Stat("/var/log/audit/audit.log"); err == nil && st.Size() > 1024*1024*1024 {
		out = append(out, Finding{
			ID:          "audit.auditd.log_oversized",
			Category:    "audit",
			Severity:    SeverityLow,
			Title:       "audit.log over 1 GiB",
			Description: "Audit log on disk has grown past 1 GiB without rotation. Likely either the rotate policy in /etc/audit/auditd.conf is too generous, or auditd hasn't been restarted to pick up new settings.",
			Evidence:    "/var/log/audit/audit.log size " + formatSize(st.Size()),
			Remediation: "Tune `max_log_file` and `num_logs` in /etc/audit/auditd.conf, then `service auditd reload`.",
		})
	}
	return out, nil
}

// formatSize returns a coarse human-readable byte count.
func formatSize(n int64) string {
	const (
		kib = 1024
		mib = 1024 * 1024
		gib = 1024 * 1024 * 1024
	)
	switch {
	case n >= gib:
		return fmt.Sprintf("%.1f GiB", float64(n)/gib)
	case n >= mib:
		return fmt.Sprintf("%.1f MiB", float64(n)/mib)
	case n >= kib:
		return fmt.Sprintf("%.1f KiB", float64(n)/kib)
	}
	return fmt.Sprintf("%d B", n)
}
