//go:build linux

package security

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func init() {
	Register(&mountOptionsCheck{})
}

// mountOptionsCheck verifies that the standard "small, world-shared,
// not-supposed-to-host-binaries" filesystems carry the recommended
// hardening mount options. Mostly tracks CIS 1.1.x.
//
// Probes /proc/mounts (cheap, always present on Linux). Skips when
// the mount point isn't a separate filesystem (older installs collapse
// /tmp into the root mount) — flagging that case is its own
// recommendation, not a missing-option finding.
type mountOptionsCheck struct{}

func (mountOptionsCheck) ID() string                        { return "fs.mount_options" }
func (mountOptionsCheck) Category() string                  { return "filesystem" }
func (mountOptionsCheck) Applicable(_ context.Context) bool { return fileExists("/proc/mounts") }
func (mountOptionsCheck) Metadata() CheckMetadata {
	return CheckMetadata{
		Title: "Mount options on /tmp, /var/tmp, /dev/shm, /home",
		Description: "Reads /proc/mounts and reports any of the four standard " +
			"\"shouldn't host binaries / shouldn't allow privilege bits\" filesystems " +
			"that are missing recommended hardening options (nodev, nosuid, noexec — " +
			"with /home only requiring nodev). Containers inherit the host's flags " +
			"so the same advice applies inside privileged container hosts.",
		References: []string{"CIS 1.1.2-1.1.18"},
	}
}

type mountSpec struct {
	mountpoint string
	required   []string // option names that MUST be present
	severity   string
}

var mountSpecs = []mountSpec{
	{"/tmp", []string{"nodev", "nosuid", "noexec"}, SeverityMedium},
	{"/var/tmp", []string{"nodev", "nosuid", "noexec"}, SeverityMedium},
	{"/dev/shm", []string{"nodev", "nosuid", "noexec"}, SeverityMedium},
	{"/home", []string{"nodev"}, SeverityLow},
}

func (mountOptionsCheck) Run(_ context.Context) ([]Finding, error) {
	mounts, err := readMounts()
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, spec := range mountSpecs {
		opts, mounted := mounts[spec.mountpoint]
		if !mounted {
			// Not a separate filesystem — surface as a low-severity
			// observation. /tmp on the root partition IS a missed
			// hardening opportunity, but it's not the same class as
			// "mounted with the wrong flags".
			if spec.mountpoint == "/tmp" || spec.mountpoint == "/var/tmp" {
				out = append(out, Finding{
					ID:       "fs.mount." + cleanMountID(spec.mountpoint) + ".not_separate",
					Category: "filesystem",
					Severity: SeverityLow,
					Title:    spec.mountpoint + " is not a separate filesystem",
					Description: spec.mountpoint + " sharing the root partition means it inherits " +
						"the root mount's flags — typically without nodev/nosuid/noexec. CIS " +
						"recommends giving each of these directories its own mount (a tmpfs is " +
						"fine) so the hardening flags can be applied independently.",
					Evidence:    spec.mountpoint + " not present in /proc/mounts as a distinct mount",
					Remediation: "Create a tmpfs entry in /etc/fstab for " + spec.mountpoint + " with the recommended options, e.g. `tmpfs " + spec.mountpoint + " tmpfs defaults,nodev,nosuid,noexec 0 0` and remount.",
				})
			}
			continue
		}
		var missing []string
		for _, want := range spec.required {
			if !hasOpt(opts, want) {
				missing = append(missing, want)
			}
		}
		if len(missing) == 0 {
			continue
		}
		out = append(out, Finding{
			ID:          "fs.mount." + cleanMountID(spec.mountpoint),
			Category:    "filesystem",
			Severity:    spec.severity,
			Title:       fmt.Sprintf("%s missing hardening mount options", spec.mountpoint),
			Description: spec.mountpoint + " should mount with " + strings.Join(spec.required, ","),
			Evidence:    fmt.Sprintf("%s currently mounted with %q (missing %s)", spec.mountpoint, opts, strings.Join(missing, ",")),
			Remediation: fmt.Sprintf("Edit /etc/fstab to add %s to the %s entry and run `mount -o remount %s`.", strings.Join(missing, ","), spec.mountpoint, spec.mountpoint),
		})
	}
	return out, nil
}

// readMounts parses /proc/mounts into a map of mountpoint → comma-
// separated options. Last-write-wins for duplicate mountpoints
// (overlay layouts) so the operator-facing flags are reflected.
func readMounts() (map[string]string, error) {
	b, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// /proc/mounts: device mountpoint fstype options ...
		out[fields[1]] = fields[3]
	}
	return out, nil
}

func hasOpt(opts, want string) bool {
	for _, o := range strings.Split(opts, ",") {
		if o == want {
			return true
		}
	}
	return false
}

func cleanMountID(p string) string {
	// "/var/tmp" → "var_tmp", "/dev/shm" → "dev_shm" — flat finding
	// ids that play nicely with the existing dot-separated namespace.
	p = strings.TrimPrefix(p, "/")
	return strings.ReplaceAll(p, "/", "_")
}
