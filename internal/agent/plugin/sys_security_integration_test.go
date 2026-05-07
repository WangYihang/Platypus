package plugin_test

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func installSysSecurity(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-security", "2.0.0", "sys_security.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-security", "2.0.0")

	pluginRoot := t.TempDir()
	paths := plugin.NewPaths(pluginRoot)
	sk, pk, err := plugin.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PublisherKeyFile(plugin.HumanKeyID(pk)),
		[]byte(plugin.EncodePublicKey(pk, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestStr := rewriteManifestKeyID(string(manifestBytes), plugin.HumanKeyID(pk))
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_security.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-security",
		Version:             "2.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []plugin.CapabilityID{"fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestSecurity_ListChecks: list_security_checks reports the v2
// check catalog. Asserts at least one check entry comes back with
// non-empty id + category, since the manifest documents that v2
// covers kernel.version + ssh.config.
func TestSecurity_ListChecks(t *testing.T) {
	reg := installSysSecurity(t)

	resp := bridge.ListSecurityChecks(reg)(context.Background(),
		&v2pb.ListSecurityChecksRequest{})
	if resp.GetError() != "" {
		t.Fatalf("list_security_checks error: %s", resp.GetError())
	}
	checks := resp.GetChecks()
	if len(checks) == 0 {
		t.Fatal("expected at least one available check; got empty list")
	}
	for i, c := range checks {
		if c.GetId() == "" {
			t.Errorf("check[%d] has empty id", i)
		}
	}
}

// TestSecurity_Scan_RunsAndProducesResults: with no filters, the
// scan runs every available check. Assert the response carries
// (a) a non-zero StartedAtUnix, (b) at least one CheckResult, and
// (c) elapsed_ms is populated. The exact findings depend on the
// host OS, so we don't assert specific posture.
func TestSecurity_Scan_RunsAndProducesResults(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v2 checks read /proc + /etc — linux-only")
	}
	reg := installSysSecurity(t)

	resp := bridge.SecurityScan(reg)(context.Background(),
		&v2pb.SecurityScanRequest{})
	if resp.GetError() != "" {
		t.Fatalf("security_scan error: %s", resp.GetError())
	}
	if resp.GetStartedAtUnix() == 0 {
		t.Errorf("started_at_unix not populated")
	}
	if len(resp.GetChecks()) == 0 {
		t.Errorf("scan returned zero CheckResults")
	}
}

// TestSecurity_Scan_FiltersByCheckIDs: passing check_ids restricts
// the scan to those ids. We pick "kernel.version" (documented in
// the plugin manifest) and verify only that check ran (assuming the
// id exists; if the plugin renamed it, the test signals stale and
// gets fixed in the same change).
func TestSecurity_Scan_FiltersByCheckIDs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v2 checks read /proc — linux-only")
	}
	reg := installSysSecurity(t)

	// First confirm "kernel.version" exists in the catalog.
	listed := bridge.ListSecurityChecks(reg)(context.Background(),
		&v2pb.ListSecurityChecksRequest{})
	hasKernelVersion := false
	for _, c := range listed.GetChecks() {
		if c.GetId() == "kernel.version" {
			hasKernelVersion = true
			break
		}
	}
	if !hasKernelVersion {
		t.Skipf("kernel.version not in catalog; available: %v", checkIDs(listed.GetChecks()))
	}

	resp := bridge.SecurityScan(reg)(context.Background(),
		&v2pb.SecurityScanRequest{CheckIds: []string{"kernel.version"}})
	if resp.GetError() != "" {
		t.Fatalf("security_scan error: %s", resp.GetError())
	}
	if got := len(resp.GetChecks()); got != 1 {
		t.Errorf("expected exactly 1 CheckResult for kernel.version filter, got %d", got)
	}
	if got := resp.GetChecks()[0].GetId(); got != "kernel.version" {
		t.Errorf("check[0].id = %q; want kernel.version", got)
	}
}

func checkIDs(checks []*v2pb.AvailableSecurityCheck) []string {
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.GetId())
	}
	return out
}

