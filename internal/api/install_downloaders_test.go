package api

import (
	"sort"
	"strings"
	"testing"
)

// TestInstallDownloaders_RenderShape covers every entry in the
// registry: the rendered string must contain the install URL, a
// per-tool "skip TLS verification" flag (the install endpoint may be
// self-signed on first-boot deployments), and the expected pipe-to-
// shell suffix. Adding a new downloader without updating this table
// will fail loudly rather than ship an under-tested template.
func TestInstallDownloaders_RenderShape(t *testing.T) {
	const url = "https://example.test:9443/api/v1/install/dl_abc.def"
	cases := []struct {
		name           string
		insecureMarker string // unique substring proving the no-verify flag is present
		suffix         string // last few chars to confirm the pipe is correctly oriented
		// extraMarker is checked when non-empty. We use it on Windows
		// templates to lock in the TLS 1.2 force — Windows PS 5.1
		// defaults to TLS 1.0 and the server rejects sub-1.2.
		extraMarker string
	}{
		{name: "curl", insecureMarker: "-k ", suffix: "| sh"},
		{name: "wget", insecureMarker: "--no-check-certificate", suffix: "| sh"},
		{name: "python3", insecureMarker: "_create_unverified_context", suffix: "| sh"},
		{name: "php", insecureMarker: "verify_peer'=>false", suffix: "| sh"},
		{name: "ruby", insecureMarker: "ssl_verify_mode: 0", suffix: "| sh"},
		{name: "powershell", insecureMarker: "ServerCertificateValidationCallback", suffix: "| iex\"", extraMarker: "SecurityProtocolType]::Tls12"},
		{name: "pwsh", insecureMarker: "-SkipCertificateCheck", suffix: "| iex\"", extraMarker: "SecurityProtocolType]::Tls12"},
	}
	byName := make(map[string]downloader, len(installDownloaders))
	for _, d := range installDownloaders {
		byName[d.name] = d
	}
	for _, c := range cases {
		d, ok := byName[c.name]
		if !ok {
			t.Errorf("downloader %q missing from registry", c.name)
			continue
		}
		got := d.render(url)
		if !strings.Contains(got, url) {
			t.Errorf("%s: rendered command does not contain URL: %q", c.name, got)
		}
		if !strings.Contains(got, c.insecureMarker) {
			t.Errorf("%s: missing insecure-skip marker %q in: %q",
				c.name, c.insecureMarker, got)
		}
		if !strings.HasSuffix(got, c.suffix) {
			t.Errorf("%s: expected suffix %q, got %q", c.name, c.suffix, got)
		}
		if c.extraMarker != "" && !strings.Contains(got, c.extraMarker) {
			t.Errorf("%s: missing extra marker %q in: %q",
				c.name, c.extraMarker, got)
		}
	}
}

// TestRenderInstallCommandsFor_FamilyFiltering covers the OS-aware
// dispatch: a unix target must surface only unix-family downloaders
// and vice versa. Also pins the "first family entry wins" rule for
// the legacy install_command default field.
func TestRenderInstallCommandsFor_FamilyFiltering(t *testing.T) {
	const url = "https://example.test/api/v1/install/dl_abc.def"

	t.Run("unix", func(t *testing.T) {
		cmds, def := renderInstallCommandsFor(url, "linux")
		want := []string{"curl", "wget", "python3", "php", "ruby"}
		assertKeys(t, cmds, want)
		if !strings.HasPrefix(def, "curl ") {
			t.Errorf("expected curl to be the unix default, got %q", def)
		}
	})

	t.Run("darwin", func(t *testing.T) {
		// macOS targets are unix-family — the whole point of the
		// downloader picker is to give macOS users an alternative
		// when their LibreSSL curl is broken.
		cmds, _ := renderInstallCommandsFor(url, "darwin")
		assertKeys(t, cmds, []string{"curl", "wget", "python3", "php", "ruby"})
	})

	t.Run("empty", func(t *testing.T) {
		// Empty target_os == "operator skipped the OS picker"; the
		// distributor falls back to the auto-detecting POSIX script,
		// so we surface unix downloaders.
		cmds, _ := renderInstallCommandsFor(url, "")
		assertKeys(t, cmds, []string{"curl", "wget", "python3", "php", "ruby"})
	})

	t.Run("windows", func(t *testing.T) {
		cmds, def := renderInstallCommandsFor(url, "windows")
		assertKeys(t, cmds, []string{"powershell", "pwsh"})
		if !strings.HasPrefix(def, "powershell ") {
			t.Errorf("expected powershell to be the windows default, got %q", def)
		}
	})
}

func assertKeys(t *testing.T, m map[string]string, want []string) {
	t.Helper()
	got := make([]string, 0, len(m))
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)
	if strings.Join(got, ",") != strings.Join(wantSorted, ",") {
		t.Errorf("downloader keys mismatch\n  want: %v\n  got:  %v", wantSorted, got)
	}
}
