package api

import (
	"sort"
	"strings"
	"testing"
)

// TestInstallDownloaders_RenderShape covers every entry in the
// registry across the full {script, bundle} × {insecure, strict}
// matrix:
//
//   - the insecure flavour MUST contain a per-tool skip-cert marker
//   - the strict flavour MUST NOT contain that marker
//   - script commands end with "| sh" (unix) or "| iex\"" (windows)
//   - bundle commands wrap the fetch in `platypus-agent` (unix) or
//     a PowerShell `& platypus-agent.exe (...)` invocation (windows)
//   - Windows templates additionally lock in the TLS 1.2 protocol
//     force regardless of trust mode (protocol-layer fix, not a
//     verification one)
func TestInstallDownloaders_RenderShape(t *testing.T) {
	const url = "https://example.test:9443/api/v1/install/dl_abc.def"
	cases := []struct {
		name           string
		insecureMarker string // substring that MUST be present when insecure=true
		scriptSuffix   string // last few chars of the script-shape rendering
		bundleMarker   string // substring that proves the agent invocation is present
		// extraMarker is checked when non-empty in BOTH flavours and
		// BOTH shapes. We use it on Windows to lock in the TLS 1.2
		// force (independent of trust mode).
		extraMarker string
	}{
		{name: "curl", insecureMarker: "-k ", scriptSuffix: "| sh", bundleMarker: `platypus-agent "$(curl `},
		{name: "wget", insecureMarker: "--no-check-certificate", scriptSuffix: "| sh", bundleMarker: `platypus-agent "$(wget `},
		{name: "python3", insecureMarker: "_create_unverified_context", scriptSuffix: "| sh", bundleMarker: `platypus-agent "$(python3 `},
		{name: "php", insecureMarker: "verify_peer'=>false", scriptSuffix: "| sh", bundleMarker: `platypus-agent "$(php `},
		{name: "ruby", insecureMarker: "ssl_verify_mode: 0", scriptSuffix: "| sh", bundleMarker: `platypus-agent "$(ruby `},
		{name: "powershell", insecureMarker: "ServerCertificateValidationCallback", scriptSuffix: "| iex\"", bundleMarker: "& platypus-agent.exe (Invoke-RestMethod", extraMarker: "SecurityProtocolType]::Tls12"},
		{name: "pwsh", insecureMarker: "-SkipCertificateCheck", scriptSuffix: "| iex\"", bundleMarker: "& platypus-agent.exe (Invoke-RestMethod", extraMarker: "SecurityProtocolType]::Tls12"},
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

		// --- script / insecure ---
		scriptIns := d.render(url, renderOpts{asBundle: false, insecure: true})
		assertContains(t, c.name+"/script/insecure URL", scriptIns, url)
		assertContains(t, c.name+"/script/insecure marker", scriptIns, c.insecureMarker)
		assertSuffix(t, c.name+"/script/insecure suffix", scriptIns, c.scriptSuffix)
		if c.extraMarker != "" {
			assertContains(t, c.name+"/script/insecure extra", scriptIns, c.extraMarker)
		}

		// --- script / strict ---
		scriptStrict := d.render(url, renderOpts{asBundle: false, insecure: false})
		assertContains(t, c.name+"/script/strict URL", scriptStrict, url)
		if strings.Contains(scriptStrict, c.insecureMarker) {
			t.Errorf("%s/script/strict: skip-cert marker %q leaked into strict variant: %q",
				c.name, c.insecureMarker, scriptStrict)
		}
		assertSuffix(t, c.name+"/script/strict suffix", scriptStrict, c.scriptSuffix)
		if c.extraMarker != "" {
			assertContains(t, c.name+"/script/strict extra", scriptStrict, c.extraMarker)
		}

		// --- bundle / insecure ---
		bundleIns := d.render(url, renderOpts{asBundle: true, insecure: true})
		assertContains(t, c.name+"/bundle/insecure URL", bundleIns, url)
		assertContains(t, c.name+"/bundle/insecure marker", bundleIns, c.insecureMarker)
		assertContains(t, c.name+"/bundle/insecure agent", bundleIns, c.bundleMarker)
		// Bundle must NOT have | sh / | iex — that would feed the
		// pinst_ token to a shell instead of the agent CLI.
		if strings.HasSuffix(bundleIns, " | sh") || strings.Contains(bundleIns, "| iex") {
			t.Errorf("%s/bundle/insecure: looks like a script shape, not a bundle shape: %q", c.name, bundleIns)
		}
		if c.extraMarker != "" {
			assertContains(t, c.name+"/bundle/insecure extra", bundleIns, c.extraMarker)
		}

		// --- bundle / strict ---
		bundleStrict := d.render(url, renderOpts{asBundle: true, insecure: false})
		assertContains(t, c.name+"/bundle/strict URL", bundleStrict, url)
		if strings.Contains(bundleStrict, c.insecureMarker) {
			t.Errorf("%s/bundle/strict: skip-cert marker %q leaked into strict variant: %q",
				c.name, c.insecureMarker, bundleStrict)
		}
		assertContains(t, c.name+"/bundle/strict agent", bundleStrict, c.bundleMarker)
		if c.extraMarker != "" {
			assertContains(t, c.name+"/bundle/strict extra", bundleStrict, c.extraMarker)
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

// TestRenderBundleCommandsFor_KeysMatchScript locks in the
// invariant the FE relies on: bundle and script maps for the same
// target_os have identical key sets, so the wizard's downloader
// picker can read keys from either one without the picker contents
// drifting between the two tabs.
func TestRenderBundleCommandsFor_KeysMatchScript(t *testing.T) {
	const url = "https://example.test/api/v1/install/dl_abc.def"
	for _, target := range []string{"linux", "darwin", "windows", ""} {
		scriptInsecure, scriptStrict, _, _ := renderInstallCommandsFor(url, target)
		bundleInsecure, bundleStrict, _, _ := renderBundleCommandsFor(url, target)
		assertKeys(t, bundleInsecure, mapKeys(scriptInsecure))
		assertKeys(t, bundleStrict, mapKeys(scriptStrict))
	}
}

func assertContains(t *testing.T, label, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s: missing %q in: %q", label, needle, haystack)
	}
}

func assertSuffix(t *testing.T, label, s, suffix string) {
	t.Helper()
	if !strings.HasSuffix(s, suffix) {
		t.Errorf("%s: expected suffix %q, got %q", label, suffix, s)
	}
}

func assertKeys(t *testing.T, m map[string]string, want []string) {
	t.Helper()
	got := mapKeys(m)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)
	if strings.Join(got, ",") != strings.Join(wantSorted, ",") {
		t.Errorf("downloader keys mismatch\n  want: %v\n  got:  %v", wantSorted, got)
	}
}

func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
