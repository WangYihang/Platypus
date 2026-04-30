//go:build linux

package security

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	Register(&kernelCVECheck{})
}

// kernelCVECheck flags hosts whose `/proc/sys/kernel/osrelease` falls
// inside a published CVE's vulnerable range and outside any of the
// branches known to carry the upstream fix. It's a HEURISTIC — major
// distros (Ubuntu / RHEL / SUSE / Debian) backport security patches
// onto the same upstream tag without bumping `uname -r`, so a finding
// here means "this kernel may be vulnerable" not "this kernel IS
// vulnerable". The remediation copy nudges operators toward the
// distro's authoritative changelog (`apt changelog linux | grep
// CVE-XXXX`, `rpm -q --changelog kernel | grep CVE-XXXX`) before
// treating a hit as actionable.
//
// The CVE table is intentionally short and curated — only LPE-class
// vulnerabilities with public exploits or active exploitation. Adding
// a new entry is one struct literal in `kernelCVEs` below.

type kernelCVECheck struct{}

func (kernelCVECheck) ID() string                        { return "kernel.cve" }
func (kernelCVECheck) Category() string                  { return "kernel" }
func (kernelCVECheck) Applicable(_ context.Context) bool { return true }
func (kernelCVECheck) Metadata() CheckMetadata {
	cves := make([]string, 0, len(kernelCVEs))
	for _, c := range kernelCVEs {
		cves = append(cves, c.cve)
	}
	return CheckMetadata{
		Title: "Known kernel privilege-escalation CVEs",
		Description: "Compares /proc/sys/kernel/osrelease against a curated table of " +
			"recently-exploited Linux kernel LPE vulnerabilities (DirtyPipe, OverlayFS, " +
			"nf_tables UAF, Copy Fail, …). HEURISTIC — major distros backport fixes onto " +
			"the same upstream tag without bumping uname -r, so a finding here means " +
			"\"may be vulnerable\" not \"is vulnerable\". Confirm with the distro changelog " +
			"(apt changelog linux / rpm -q --changelog kernel) before treating a hit as " +
			"actionable. For authoritative judgement, layer in a tool that consumes OVAL " +
			"feeds (vuls, openscap-scanner, distro security trackers).",
		References: append([]string{"see entries below"}, cves...),
	}
}

// kernelCVE is one row in the table. `vulnFrom` is the lowest
// known-vulnerable version, `fixedIn` is a list of branch fixes —
// "if I'm on the same major.minor as one of these, the kernel is
// patched at >= that point release". `fixedAfter`, when non-zero,
// caps the table-wide vulnerability at a major.minor — every kernel
// strictly newer than that is unaffected (e.g. nf_tables fix landed
// in upstream 6.7+, so 6.8+ is fine without checking branch fixes).
type kernelCVE struct {
	cve         string
	title       string
	severity    string
	vulnFrom    [3]int   // {major, minor, patch} introduced
	fixedIn     [][3]int // patched point releases per LTS branch
	fixedAfter  [3]int   // upstream-fixed cutoff (zero = no cutoff)
	references  []string
	description string
	remediation string
}

// kernelCVEs is the curated table. Add a new entry by filling in one
// struct literal — no other code changes needed.
//
// Sources audited at the time of writing; double-check the distro
// changelog before treating a hit as definitive (see Metadata).
var kernelCVEs = []kernelCVE{
	{
		cve: "CVE-2022-0847", title: "DirtyPipe (pipe_buffer.flags uninitialised)",
		severity: SeverityHigh,
		vulnFrom: [3]int{5, 8, 0},
		fixedIn: [][3]int{
			{5, 10, 102}, // 5.10 LTS
			{5, 15, 25},  // 5.15 LTS
			{5, 16, 11},  // 5.16
		},
		fixedAfter:  [3]int{5, 16, 11},
		references:  []string{"CVE-2022-0847"},
		description: "Linux kernel 5.8+ ships a uninitialised pipe_buffer.flags read that lets unprivileged users overwrite read-only files in the page cache (including SUID binaries and /etc/shadow). Used in real-world LPE exploits.",
		remediation: "Upgrade to a kernel ≥ 5.16.11 (or distro backport: Ubuntu 5.4.0-105 / RHEL kernel-4.18.0-348.20.1.el8_5 / Debian linux 5.10.92-2). Reboot afterwards.",
	},
	{
		cve: "CVE-2023-0386", title: "OverlayFS UID-mapping LPE",
		severity: SeverityHigh,
		vulnFrom: [3]int{5, 11, 0},
		fixedIn: [][3]int{
			{5, 15, 91}, // 5.15 LTS
			{6, 1, 9},   // 6.1 LTS
		},
		fixedAfter:  [3]int{6, 2, 0},
		references:  []string{"CVE-2023-0386", "CISA KEV"},
		description: "Overlayfs failed to validate UID/GID mappings of files copied from a 'lower' to the 'upper' directory, letting unprivileged users smuggle root-owned SUID binaries up. PoC public; CISA Known Exploited Vulnerabilities list.",
		remediation: "Upgrade to ≥ 6.2-rc6 or a distro backport (Ubuntu 5.19.0-41 / 5.15.0-70, Debian 5.10.179-1). Until patched, mount /tmp with nosuid as a partial mitigation.",
	},
	{
		cve: "CVE-2024-1086", title: "nf_tables nft_verdict_init double-free",
		severity: SeverityCritical,
		vulnFrom: [3]int{5, 14, 0},
		fixedIn: [][3]int{
			{5, 15, 149}, // 5.15 LTS
			{6, 1, 76},   // 6.1 LTS
			{6, 6, 15},   // 6.6 LTS
		},
		fixedAfter:  [3]int{6, 7, 0},
		references:  []string{"CVE-2024-1086", "CISA KEV"},
		description: "Use-after-free in netfilter's nft_verdict_init() reachable from an unprivileged network namespace. Universal LPE exploit (Notselwyn) reports >99% success on KernelCTF; observed in active ransomware campaigns. Mitigate immediately by patching or by `sysctl -w kernel.unprivileged_userns_clone=0`.",
		remediation: "Upgrade kernel to a fixed release (≥ 5.15.149 / 6.1.76 / 6.6.15) or, as an interim mitigation, disable user namespaces with `sysctl -w kernel.unprivileged_userns_clone=0` (Debian/Ubuntu) or remove CAP_NET_ADMIN from non-root processes.",
	},
	{
		cve: "CVE-2026-31431", title: "Copy Fail (AF_ALG AEAD page-cache corruption)",
		severity: SeverityHigh,
		// 2017 commit 72548b093ee3 introduced the bug; conservatively
		// treat every modern kernel as potentially vulnerable until a
		// fixed-version table emerges from upstream.
		vulnFrom:    [3]int{4, 10, 0},
		fixedIn:     nil, // upstream fix landed; distro backport status varies, table will fill in
		fixedAfter:  [3]int{0, 0, 0},
		references:  []string{"CVE-2026-31431"},
		description: "The Linux crypto subsystem's in-place AEAD optimisation (AF_ALG) writes 4 bytes past the destination scatterlist via authencesn, corrupting the page cache of any readable file the attacker can splice in. Reaches root from an unprivileged user; PoC public.",
		remediation: "Apply the distro kernel update reverting AF_ALG AEAD to out-of-place (upstream commit a664bf3d603d). As a hardening interim, prevent unprivileged AF_ALG access (load `socket` LSM rule or disable CONFIG_CRYPTO_USER_API_AEAD).",
	},
}

func (kernelCVECheck) Run(_ context.Context) ([]Finding, error) {
	raw, err := readKernelRelease()
	if err != nil {
		return nil, err
	}
	major, minor, patch, ok := parseKernelVersion(raw)
	if !ok {
		return nil, fmt.Errorf("unable to parse kernel release %q", raw)
	}

	var out []Finding
	for _, e := range kernelCVEs {
		if !cveAffectsKernel(e, major, minor, patch) {
			continue
		}
		out = append(out, Finding{
			ID:       "kernel.cve." + strings.ToLower(strings.ReplaceAll(e.cve, "-", "_")),
			Category: "kernel",
			Severity: e.severity,
			Title:    e.cve + " — " + e.title,
			Description: e.description + "\n\nNOTE: this match is based on " +
				"upstream kernel version only. Major distros backport security " +
				"fixes without bumping uname -r, so this host MAY already be " +
				"patched even though the kernel string falls in the vulnerable " +
				"range. Verify with `apt changelog linux | grep " + e.cve + "` " +
				"(Debian/Ubuntu) or `rpm -q --changelog kernel | grep " + e.cve + "` " +
				"(RHEL/Rocky/Fedora) before treating this as definitive.",
			Evidence:    fmt.Sprintf("kernel %s falls in the published vulnerable range for %s", raw, e.cve),
			Remediation: e.remediation,
			References:  append([]string(nil), e.references...),
		})
	}
	return out, nil
}

// cveAffectsKernel decides whether the host's kernel sits inside the
// CVE's published vulnerable window. Logic:
//
//  1. If the host is older than the introducing version → not affected.
//  2. If `fixedAfter` is non-zero AND the host is at-or-newer than it
//     → not affected (upstream-wide cutoff).
//  3. Otherwise, look up the host's branch (matching major.minor)
//     in the per-branch fix table; if a fix exists for this branch
//     and the host's patch is at-or-newer → not affected.
//  4. Anything else → affected.
func cveAffectsKernel(e kernelCVE, maj, min, pat int) bool {
	if compareVersion(maj, min, pat, e.vulnFrom[0], e.vulnFrom[1], e.vulnFrom[2]) < 0 {
		return false
	}
	if (e.fixedAfter != [3]int{}) &&
		compareVersion(maj, min, pat, e.fixedAfter[0], e.fixedAfter[1], e.fixedAfter[2]) >= 0 {
		return false
	}
	for _, fix := range e.fixedIn {
		if maj == fix[0] && min == fix[1] && pat >= fix[2] {
			return false
		}
	}
	return true
}

