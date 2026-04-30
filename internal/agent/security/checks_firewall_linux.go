//go:build linux

package security

import (
	"context"
	"os"
	"strings"
)

func init() {
	Register(&firewallCheck{})
}

// firewallCheck reports whether *some* firewall is active on the
// host. Linux ships three layers (iptables / nftables / bpfilter)
// plus several frontends (ufw / firewalld / iptables-save) — the
// check tolerates all of them and surfaces a finding only when none
// of the well-known signals indicate any rules are loaded.
//
// We deliberately don't try to assert "the right rules are loaded"
// — that's deeply policy-specific (web tier vs DB tier vs jumphost
// have radically different correct answers). The signal here is just
// "is there ANY filter at all in front of this box".
type firewallCheck struct{}

func (firewallCheck) ID() string                        { return "network.firewall" }
func (firewallCheck) Category() string                  { return "network" }
func (firewallCheck) Applicable(_ context.Context) bool { return dirExists("/proc/net") }
func (firewallCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "Host firewall presence (iptables / nftables / ufw / firewalld)",
		Description: "Reports a finding only when ALL of the well-known firewall " +
			"signals are absent — the check is intentionally permissive, surfacing " +
			"the \"no firewall at all\" case while staying quiet on hosts that ship " +
			"any flavour of packet filter. Reading the actual ruleset typically " +
			"needs root and the in-tree probe avoids it on purpose; verify with " +
			"`nft list ruleset` or `iptables-save` when this fires.",
		References: []string{"CIS 3.5"},
	}
}

func (firewallCheck) Run(_ context.Context) ([]Finding, error) {
	if hasAnyFirewallSignal() {
		return nil, nil
	}
	return []Finding{{
		ID:       "network.firewall.absent",
		Category: "network",
		Severity: SeverityMedium,
		Title:    "No active host firewall detected",
		Description: "None of /proc/net/ip_tables_names, /proc/net/nf_tables, " +
			"/sys/module/nf_tables, /sys/module/ip_tables, or known firewall daemons " +
			"(firewalld / ufw) are present. The host appears to rely entirely on " +
			"upstream network controls (cloud SG, switch ACLs). That can be a " +
			"deliberate posture, but on operator-managed hosts a local default-deny " +
			"is the recommended last line of defence.",
		Evidence:    "no firewall kernel module or daemon detected",
		Remediation: "Pick the frontend your distro prefers and apply a default-deny baseline: `ufw enable && ufw default deny incoming` (Debian/Ubuntu), `systemctl enable --now firewalld` (RHEL/Fedora), or write an explicit nftables ruleset.",
	}}, nil
}

// hasAnyFirewallSignal returns true if any of the standard probes
// indicates a filter is loaded. Probes are cheap reads; no shell-out.
func hasAnyFirewallSignal() bool {
	// /proc/net/ip_tables_names lists tables when the iptables module
	// is loaded AND has at least one table. A loaded-but-empty
	// iptables shows the file but with empty content — we treat that
	// as "active" because the module itself indicates intent.
	if fileExists("/proc/net/ip_tables_names") {
		return true
	}
	if fileExists("/proc/net/nf_tables") {
		return true
	}
	// Modern kernels expose loaded modules as directories under
	// /sys/module. Either name being present means the in-kernel
	// hooks exist regardless of whether userspace tools are
	// installed.
	for _, m := range []string{
		"/sys/module/nf_tables",
		"/sys/module/ip_tables",
		"/sys/module/iptable_filter",
	} {
		if dirExists(m) {
			return true
		}
	}
	// Userspace daemons. Either being alive is sufficient signal —
	// we don't need to read their state, just confirm someone owns
	// the policy.
	if anyProcessNamed("firewalld", "ufw", "nft") {
		return true
	}
	// Last-ditch: nftables exposes /proc/net/nf_conntrack on systems
	// where conntrack is loaded as part of the firewall path.
	if b, err := os.ReadFile("/proc/net/protocols"); err == nil {
		s := string(b)
		if strings.Contains(s, "NETLINK") && strings.Contains(s, "PACKET") {
			// netlink + packet alone don't prove a firewall; skip
			// without returning true. Kept for future enrichment.
			_ = s
		}
	}
	return false
}
