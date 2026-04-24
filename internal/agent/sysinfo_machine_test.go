package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestClassifyVirt(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"none":         "",
		"docker":       "container",
		"LXC":          "container",
		"wsl":          "container",
		"wsl2":         "container",
		"containerd":   "container",
		"kvm":          "vm",
		"vmware":       "vm",
		"virtualbox":   "vm",
		"hyperv":       "vm",
		"Xen":          "vm",
		"parallels":    "vm",
		"bhyve":        "vm",
		"somethingnew": "",
	}
	for in, want := range cases {
		if got := classifyVirt(in); got != want {
			t.Errorf("classifyVirt(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestChassisCategory(t *testing.T) {
	cases := map[string]string{
		"":    "unknown",
		"10":  "laptop",
		"9":   "laptop",
		"8":   "laptop",
		"30":  "laptop",
		"31":  "laptop",
		"32":  "laptop",
		"3":   "desktop",
		"6":   "desktop",
		"7":   "desktop",
		"13":  "desktop",
		"17":  "bare_metal",
		"23":  "bare_metal",
		"25":  "bare_metal",
		"28":  "bare_metal",
		"99":  "unknown",
	}
	for in, want := range cases {
		if got := chassisCategory(in); got != want {
			t.Errorf("chassisCategory(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestDetectContainer drives detectContainer against a synthetic
// filesystem root in t.TempDir() so we don't depend on the test
// host actually being in a container. Linux-only: the function is
// a no-op on other platforms.
func TestDetectContainer(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("container markers only exist on Linux")
	}

	t.Run("dockerenv marker", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".dockerenv"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		name, ok := detectContainer(root)
		if !ok || name != "docker" {
			t.Fatalf("detectContainer = %q, %v; want (\"docker\", true)", name, ok)
		}
	})

	t.Run("cgroup kubepods", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "proc/1"), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "12:pids:/kubepods/burstable/poda0/abc123\n"
		if err := os.WriteFile(filepath.Join(root, "proc/1/cgroup"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		name, ok := detectContainer(root)
		if !ok || name != "kubepods" {
			t.Fatalf("detectContainer = %q, %v; want (\"kubepods\", true)", name, ok)
		}
	})

	t.Run("no markers", func(t *testing.T) {
		root := t.TempDir()
		name, ok := detectContainer(root)
		if ok || name != "" {
			t.Fatalf("detectContainer on empty root = %q, %v; want (\"\", false)", name, ok)
		}
	})
}

func TestApplyAppleModelHeuristic(t *testing.T) {
	cases := []struct {
		model       string
		wantName    string
		wantVendor  string
		wantChassis string
	}{
		{"MacBookPro18,1", "MacBookPro18,1", "Apple Inc.", "10"},
		{"macbookair10,1", "macbookair10,1", "Apple Inc.", "10"},
		{"iMac21,1", "iMac21,1", "Apple Inc.", "3"},
		{"Macmini9,1", "Macmini9,1", "Apple Inc.", "3"},
		{"MacPro7,1", "MacPro7,1", "Apple Inc.", "3"},
		{"MacStudio1,1", "MacStudio1,1", "Apple Inc.", "3"},
		{"UnknownModel99", "UnknownModel99", "Apple Inc.", ""},
	}
	for _, c := range cases {
		s := &machineSnapshot{}
		applyAppleModelHeuristic(s, c.model)
		if s.ProductName != c.wantName {
			t.Errorf("model=%q: ProductName = %q; want %q", c.model, s.ProductName, c.wantName)
		}
		if s.ProductVendor != c.wantVendor {
			t.Errorf("model=%q: ProductVendor = %q; want %q", c.model, s.ProductVendor, c.wantVendor)
		}
		if s.ChassisType != c.wantChassis {
			t.Errorf("model=%q: ChassisType = %q; want %q", c.model, s.ChassisType, c.wantChassis)
		}
	}
}
