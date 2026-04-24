package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jaypipes/ghw"
	hostinfo "github.com/shirou/gopsutil/v4/host"
)

// machineSnapshot carries the agent's one-shot classification of its
// environment — the answer to "is this a container, a VM, a server, a
// laptop, or a desktop?" — plus the supporting hardware identity
// fields (vendor / product / BIOS / chassis type) the UI surfaces
// alongside the high-level category.
type machineSnapshot struct {
	MachineType      string // "container" | "vm" | "bare_metal" | "laptop" | "desktop" | "unknown"
	ContainerRuntime string
	ChassisType      string // DMTF SMBIOS enum as a string, e.g. "10"
	ProductVendor    string
	ProductName      string
	BIOSVendor       string
	BIOSVersion      string
}

// collectMachineSnapshot derives the machine classification and
// associated identity fields. Best-effort: any failed probe leaves
// the corresponding field empty and execution continues. The live
// gopsutil host.Info result is passed in so we don't re-read it.
func collectMachineSnapshot(info *hostinfo.InfoStat) machineSnapshot {
	s := machineSnapshot{}
	readChassisInto(&s)

	runtimeName, isContainer := detectContainer("/")
	virtRaw := ""
	if info != nil {
		virtRaw = strings.ToLower(info.VirtualizationSystem)
	}
	virtKind := classifyVirt(virtRaw)

	// Precedence: an explicit container marker (/.dockerenv, cgroup)
	// wins over gopsutil's VirtualizationSystem, which can come back
	// as "kvm" even inside a container nested in a KVM VM.
	switch {
	case isContainer:
		s.MachineType = "container"
		s.ContainerRuntime = runtimeName
	case virtKind == "container":
		s.MachineType = "container"
		s.ContainerRuntime = virtRaw
	case virtKind == "vm":
		s.MachineType = "vm"
	default:
		s.MachineType = chassisCategory(s.ChassisType)
	}
	return s
}

// detectContainer checks the well-known markers that Linux container
// runtimes leave behind. Returns (runtime, true) when a marker is
// found, or ("", false) otherwise. On non-Linux hosts the function
// always returns false — Windows/macOS containers are out of scope
// for this signal.
//
// `root` is injectable so tests can point at a fixture tree in
// t.TempDir() instead of the real filesystem.
func detectContainer(root string) (string, bool) {
	if runtime.GOOS != "linux" {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(root, ".dockerenv")); err == nil {
		return "docker", true
	}
	for _, path := range []string{"proc/1/cgroup", "proc/self/cgroup"} {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			continue
		}
		txt := strings.ToLower(string(data))
		switch {
		case strings.Contains(txt, "kubepods"):
			return "kubepods", true
		case strings.Contains(txt, "containerd"):
			return "containerd", true
		case strings.Contains(txt, "docker"):
			return "docker", true
		case strings.Contains(txt, "podman"):
			return "podman", true
		case strings.Contains(txt, "/lxc/") || strings.HasPrefix(txt, "lxc/"):
			return "lxc", true
		}
	}
	return "", false
}

// classifyVirt maps gopsutil's VirtualizationSystem string to one of
// our three coarse buckets. Empty input → "" (bare-metal / unknown).
func classifyVirt(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || v == "none" {
		return ""
	}
	switch v {
	case "docker", "lxc", "podman", "containerd", "wsl", "wsl2", "rkt":
		return "container"
	case "kvm", "qemu", "vmware", "vbox", "virtualbox", "hyperv", "xen",
		"parallels", "bhyve", "microsoft", "vmm":
		return "vm"
	}
	return ""
}

// chassisCategory maps the DMTF SMBIOS System Enclosure type (spec
// 3.3.4.1) to one of our categories. Unknown / reserved values fall
// through to "unknown".
func chassisCategory(chassis string) string {
	switch strings.TrimSpace(chassis) {
	case "8", "9", "10", "14", "30", "31", "32":
		// 8 Portable, 9 Laptop, 10 Notebook, 14 Sub Notebook, 30 Tablet,
		// 31 Convertible, 32 Detachable.
		return "laptop"
	case "3", "4", "5", "6", "7", "13", "15", "16":
		// 3 Desktop, 4 Low Profile Desktop, 5 Pizza Box, 6 Mini Tower,
		// 7 Tower, 13 All in One, 15 Space Saving, 16 Lunch Box.
		return "desktop"
	case "17", "23", "25", "28":
		// 17 Main Server Chassis, 23 Rack Mount Chassis,
		// 25 Multi-system Chassis, 28 Blade.
		return "bare_metal"
	}
	return "unknown"
}

// readChassisInto populates the chassis / product / BIOS identity
// fields on `s`. Linux + Windows go through ghw (reads /sys/class/dmi
// and WMI respectively); macOS falls back to `sysctl hw.model` since
// ghw doesn't expose the SMBIOS tables Apple doesn't fully publish.
func readChassisInto(s *machineSnapshot) {
	defer func() {
		// ghw historically panics on some oddly configured hosts
		// (empty /sys/class/dmi). Swallow and leave fields empty.
		_ = recover()
	}()

	if chassis, err := ghw.Chassis(ghw.WithDisableWarnings()); err == nil && chassis != nil {
		if chassis.Type != "" {
			s.ChassisType = chassis.Type
		}
	}
	if prod, err := ghw.Product(ghw.WithDisableWarnings()); err == nil && prod != nil {
		s.ProductVendor = firstNonEmpty(prod.Vendor, s.ProductVendor)
		s.ProductName = firstNonEmpty(prod.Name, s.ProductName)
	}
	if bios, err := ghw.BIOS(ghw.WithDisableWarnings()); err == nil && bios != nil {
		s.BIOSVendor = firstNonEmpty(bios.Vendor, s.BIOSVendor)
		s.BIOSVersion = firstNonEmpty(bios.Version, s.BIOSVersion)
	}

	if runtime.GOOS == "darwin" && (s.ChassisType == "" || s.ProductName == "") {
		applyDarwinChassisFallback(s)
	}
}

// applyAppleModelHeuristic maps a macOS `hw.model` identifier to a
// DMTF chassis type and fills ProductName / ProductVendor when
// empty. The heuristic (macbook* → notebook; iMac / Mac mini / Mac
// Pro / Mac Studio → desktop) is plenty good enough for fleet
// overview. Extracted out of applyDarwinChassisFallback so the
// unit test can exercise it on any host.
func applyAppleModelHeuristic(s *machineSnapshot, model string) {
	if model == "" {
		return
	}
	if s.ProductName == "" {
		s.ProductName = model
	}
	if s.ProductVendor == "" {
		s.ProductVendor = "Apple Inc."
	}
	if s.ChassisType != "" {
		return
	}
	low := strings.ToLower(model)
	switch {
	case strings.HasPrefix(low, "macbook"):
		s.ChassisType = "10" // Notebook
	case strings.HasPrefix(low, "imac"),
		strings.HasPrefix(low, "macmini"),
		strings.HasPrefix(low, "macpro"),
		strings.HasPrefix(low, "macstudio"):
		s.ChassisType = "3" // Desktop
	}
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
