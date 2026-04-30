package api

import (
	"sort"
	"strings"
	"testing"
)

// TestInstallDownloaders_RenderShape covers every entry in the
// registry in BOTH flavours: the insecure flavour must contain a
// per-tool skip-cert marker, the strict flavour must NOT. Both
// flavours must contain the URL and end with the expected
// pipe-to-shell suffix. Windows templates additionally lock in the
// TLS 1.2 protocol force regardless of trust mode — that's a
// protocol-layer fix, not a verification one.
func TestInstallDownloaders_RenderShape(t *testing.T) {
	const url = "https://example.test:9443/api/v1/install/dl_abc.def"
	cases := []struct {
		name           string
		insecureMarker string // substring that MUST be present when insecure=true
		suffix         string // last few chars to confirm the pipe is correctly oriented
		// extraMarker is checked when non-empty in BOTH flavours. We
		// use it on Windows templates to lock in the TLS 1.2 force
		// (independent of trust mode).
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

		// --- insecure flavour ---
		insecure := d.render(url, true)
		if !strings.Contains(insecure, url) {
			t.Errorf("%s/insecure: rendered command does not contain URL: %q", c.name, insecure)
		}
		if !strings.Contains(insecure, c.insecureMarker) {
			t.Errorf("%s/insecure: missing skip-cert marker %q in: %q",
				c.name, c.insecureMarker, insecure)
		}
		if !strings.HasSuffix(insecure, c.suffix) {
			t.Errorf("%s/insecure: expected suffix %q, got %q", c.name, c.suffix, insecure)
		}
		if c.extraMarker != "" && !strings.Contains(insecure, c.extraMarker) {
			t.Errorf("%s/insecure: missing extra marker %q in: %q",
				c.name, c.extraMarker, insecure)
		}

		// --- strict flavour: same URL + suffix, NO skip-cert marker ---
		strict := d.render(url, false)
		if !strings.Contains(strict, url) {
			t.Errorf("%s/strict: rendered command does not contain URL: %q", c.name, strict)
		}
		if strings.Contains(strict, c.insecureMarker) {
			t.Errorf("%s/strict: skip-cert marker %q leaked into the strict variant: %q",
				c.name, c.insecureMarker, strict)
		}
		if !strings.HasSuffix(strict, c.suffix) {
			t.Errorf("%s/strict: expected suffix %q, got %q", c.name, c.suffix, strict)
		}
		// TLS 1.2 force is independent of trust mode and must stay
		// in the strict variant too.
		if c.extraMarker != "" && !strings.Contains(strict, c.extraMarker) {
			t.Errorf("%s/strict: missing extra marker %q in: %q",
				c.name, c.extraMarker, strict)
		}
	}
}

// TestRenderInstallCommandsFor_FamilyFiltering covers the OS-aware
// dispatch in both flavours: a unix target must surface only
// unix-family downloaders (and vice versa) in BOTH the insecure and
// strict maps, and the family default in each flavour points at the
// same first-entry tool.
func TestRenderInstallCommandsFor_FamilyFiltering(t *testing.T) {
	const url = "https://example.test/api/v1/install/dl_abc.def"

	t.Run("unix", func(t *testing.T) {
		insecure, strict, insecureDef, strictDef := renderInstallCommandsFor(url, "linux")
		want := []string{"curl", "wget", "python3", "php", "ruby"}
		assertKeys(t, insecure, want)
		assertKeys(t, strict, want)
		if !strings.HasPrefix(insecureDef, "curl ") {
			t.Errorf("expected curl to be the unix insecure default, got %q", insecureDef)
		}
		if !strings.HasPrefix(strictDef, "curl ") {
			t.Errorf("expected curl to be the unix strict default, got %q", strictDef)
		}
		if strings.Contains(strictDef, "-k ") {
			t.Errorf("strict default leaked -k flag: %q", strictDef)
		}
	})

	t.Run("darwin", func(t *testing.T) {
		// macOS targets are unix-family — the whole point of the
		// downloader picker is to give macOS users an alternative
		// when their LibreSSL curl is broken.
		insecure, strict, _, _ := renderInstallCommandsFor(url, "darwin")
		want := []string{"curl", "wget", "python3", "php", "ruby"}
		assertKeys(t, insecure, want)
		assertKeys(t, strict, want)
	})

	t.Run("empty", func(t *testing.T) {
		// Empty target_os == "operator skipped the OS picker"; the
		// distributor falls back to the auto-detecting POSIX script,
		// so we surface unix downloaders.
		insecure, strict, _, _ := renderInstallCommandsFor(url, "")
		want := []string{"curl", "wget", "python3", "php", "ruby"}
		assertKeys(t, insecure, want)
		assertKeys(t, strict, want)
	})

	t.Run("windows", func(t *testing.T) {
		insecure, strict, insecureDef, strictDef := renderInstallCommandsFor(url, "windows")
		assertKeys(t, insecure, []string{"powershell", "pwsh"})
		assertKeys(t, strict, []string{"powershell", "pwsh"})
		if !strings.HasPrefix(insecureDef, "powershell ") {
			t.Errorf("expected powershell to be the windows insecure default, got %q", insecureDef)
		}
		if !strings.HasPrefix(strictDef, "powershell ") {
			t.Errorf("expected powershell to be the windows strict default, got %q", strictDef)
		}
		if strings.Contains(strictDef, "ServerCertificateValidationCallback") {
			t.Errorf("strict windows default leaked ServerCertificateValidationCallback: %q", strictDef)
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
