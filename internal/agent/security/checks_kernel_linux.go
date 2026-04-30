//go:build linux

package security

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func init() {
	Register(&kernelVersionCheck{})
}

// kernelVersionCheck flags hosts running a kernel that is older than
// a small built-in floor table of "minimum reasonable" major.minor
// versions per kernel branch. The table is intentionally coarse —
// this is a hardening hint, not a CVE oracle — because a per-CVE
// database is a maintenance burden that doesn't fit a static agent
// binary. Operators who want CVE-grade accuracy should layer in
// vuls / OVAL via the same RPC envelope from a dedicated checker.
type kernelVersionCheck struct{}

func (kernelVersionCheck) ID() string                        { return "kernel.version" }
func (kernelVersionCheck) Category() string                  { return "kernel" }
func (kernelVersionCheck) Applicable(_ context.Context) bool { return true }
func (kernelVersionCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "Kernel version freshness",
		Description: "Reads /proc/sys/kernel/osrelease and flags hosts whose kernel sits below " +
			"the recommended floor for its LTS branch (4.19 / 5.4 / 5.15 / 6.1). Heuristic — " +
			"distros that backport security fixes onto an older upstream tag will appear " +
			"vulnerable here even though they are patched; verify with the distro changelog " +
			"before treating a finding as definitive.",
	}
}

func (kernelVersionCheck) Run(_ context.Context) ([]Finding, error) {
	raw, err := readKernelRelease()
	if err != nil {
		return nil, err
	}
	major, minor, patch, ok := parseKernelVersion(raw)
	if !ok {
		return nil, fmt.Errorf("unable to parse kernel release %q", raw)
	}

	// Series floor table: kernels strictly older than these are
	// considered too old to ship hardening defaults the rest of the
	// scan assumes (e.g. unprivileged_userns_clone, BPF JIT
	// hardening). The 5.4 / 4.19 LTS floors track the kernel.org
	// long-term branches that are still patched as of 2026.
	type floor struct{ maj, min, pat int }
	var f floor
	switch {
	case major >= 6:
		f = floor{6, 1, 0} // anything in the 6.x series should be at least 6.1 LTS
	case major == 5 && minor >= 10:
		f = floor{5, 15, 0}
	case major == 5:
		f = floor{5, 4, 0}
	case major == 4:
		f = floor{4, 19, 0}
	default:
		// Pre-4.x is uniformly EOL; report unconditionally.
		return []Finding{{
			ID:          "kernel.version.eol",
			Category:    "kernel",
			Severity:    SeverityCritical,
			Title:       "End-of-life kernel series",
			Description: "Kernel branches earlier than 4.x are no longer maintained and miss several years of security backports.",
			Evidence:    raw,
			Remediation: "Upgrade to a current LTS kernel (6.1 or newer) and reboot.",
		}}, nil
	}

	if compareVersion(major, minor, patch, f.maj, f.min, f.pat) < 0 {
		sev := SeverityHigh
		if major < 4 {
			sev = SeverityCritical
		}
		return []Finding{{
			ID:          "kernel.version.outdated",
			Category:    "kernel",
			Severity:    sev,
			Title:       "Kernel older than minimum supported point release",
			Description: fmt.Sprintf("Running %s; the floor for the %d.%d series is %d.%d.%d. Older point releases miss vendor security backports.", raw, major, minor, f.maj, f.min, f.pat),
			Evidence:    raw,
			Remediation: fmt.Sprintf("Upgrade to %d.%d.%d or newer via the distribution's kernel package and reboot.", f.maj, f.min, f.pat),
		}}, nil
	}
	return nil, nil
}

func readKernelRelease() (string, error) {
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// parseKernelVersion extracts a major.minor.patch triple from a
// release string like "5.15.0-101-generic" or "6.1.55+". Anything
// past the third dotted component is ignored (distro suffix).
func parseKernelVersion(s string) (major, minor, patch int, ok bool) {
	core := s
	if i := strings.IndexAny(core, "-+ "); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) < 2 {
		return 0, 0, 0, false
	}
	var err error
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, false
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, 0, false
	}
	if len(parts) >= 3 {
		// Patch may carry trailing non-digits ("0rc1"); accept the
		// leading digit run.
		end := 0
		for end < len(parts[2]) && parts[2][end] >= '0' && parts[2][end] <= '9' {
			end++
		}
		if end > 0 {
			patch, _ = strconv.Atoi(parts[2][:end])
		}
	}
	return major, minor, patch, true
}

func compareVersion(aMaj, aMin, aPat, bMaj, bMin, bPat int) int {
	if aMaj != bMaj {
		return aMaj - bMaj
	}
	if aMin != bMin {
		return aMin - bMin
	}
	return aPat - bPat
}
