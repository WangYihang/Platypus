//go:build linux

package security

import (
	"context"
	"strings"
)

func init() {
	Register(&timeSyncCheck{})
}

// timeSyncCheck verifies that some flavour of time synchronisation
// is running. Drift past a few seconds breaks TLS expiry, Kerberos,
// log correlation, and most TOTP-based 2FA. CIS 2.2.1 lists chrony
// as the preferred backend; we accept any of the three common ones.
//
// Detection is process-name based. Running as root we could parse
// /run/chrony/chronyd.pid or /var/lib/systemd/timesync/clock; the
// /proc walk is cheaper and works whether or not we're root.
type timeSyncCheck struct{}

func (timeSyncCheck) ID() string                        { return "system.time_sync" }
func (timeSyncCheck) Category() string                  { return "system" }
func (timeSyncCheck) Applicable(_ context.Context) bool { return true }
func (timeSyncCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "Time synchronisation daemon presence",
		Description: "Walks /proc to confirm at least one of chronyd, " +
			"systemd-timesyncd, ntpd, openntpd, or ntpsec is running. Drift past " +
			"a few seconds breaks TLS certificate validation, Kerberos auth, log " +
			"correlation across hosts, and most TOTP 2FA. The check does NOT " +
			"verify drift magnitude — that's the timesync daemon's own remit.",
		References: []string{"CIS 2.2.1"},
	}
}

func (timeSyncCheck) Run(_ context.Context) ([]Finding, error) {
	// Names ordered by 2026-current popularity: chronyd is the
	// systemd default on most distros; systemd-timesyncd is the
	// fallback on minimal images (it answers to "systemd-timesyn"
	// because comm is capped at 16 bytes).
	candidates := []string{
		"chronyd",
		"systemd-timesyn",
		"systemd-timesyncd",
		"ntpd",
		"openntpd",
		"ntpsec",
	}
	if anyProcessNamed(candidates...) {
		return nil, nil
	}
	return []Finding{{
		ID:       "system.time_sync.absent",
		Category: "system",
		Severity: SeverityMedium,
		Title:    "No time-synchronisation daemon running",
		Description: "None of " + strings.Join(candidates, " / ") + " was found in /proc. " +
			"Without time sync, the system clock drifts; once it's a few minutes off, " +
			"TLS certificate validation, Kerberos tickets, log correlation, and TOTP " +
			"2FA all start failing. Containers that inherit clock from the host don't " +
			"need their own daemon — this finding is most relevant on bare-metal and " +
			"VM hosts.",
		Evidence:    "no time-sync daemon found in /proc",
		Remediation: "Pick the systemd-timesyncd default for minimal hosts (`timedatectl set-ntp true`) or chrony for production (`apt install chrony && systemctl enable --now chrony` / `dnf install chrony && systemctl enable --now chronyd`).",
	}}, nil
}
