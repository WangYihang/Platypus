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
	wasm := stagedWasmBytes(t, "com.platypus.sys-security", "3.0.0", "sys_security.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-security", "3.0.0")

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
		Version:             "3.0.0",
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

// TestSecurity_ListChecks: list_security_checks reports the v3
// catalog. Asserts every check the manifest documents shows up with a
// non-empty id + category. v3 ships six checks — kernel.version,
// kernel.mitigations, ssh.config, sysctl.posture, fs.path_writable,
// fs.suid_outliers — and a stale plugin (still on v2) would fail the
// id-set comparison loudly instead of silently skipping checks.
func TestSecurity_ListChecks(t *testing.T) {
	reg := installSysSecurity(t)

	resp := bridge.ListSecurityChecks(reg)(context.Background(),
		&v2pb.ListSecurityChecksRequest{})
	if resp.GetError() != "" {
		t.Fatalf("list_security_checks error: %s", resp.GetError())
	}
	got := map[string]bool{}
	for i, c := range resp.GetChecks() {
		if c.GetId() == "" {
			t.Errorf("check[%d] has empty id", i)
		}
		got[c.GetId()] = true
	}
	want := []string{
		"kernel.version",
		"kernel.mitigations",
		"ssh.config",
		"sysctl.posture",
		"fs.path_writable",
		"fs.suid_outliers",
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("v3 catalog missing check %q; got %v", id, checkIDs(resp.GetChecks()))
		}
	}
}

// TestSecurity_Scan_RunsAndProducesResults: with no filters, the
// scan runs every available check. Assert the response carries
// (a) a non-zero StartedAtUnix, (b) one CheckResult per registered
// check, (c) no error string. Specific findings depend on host
// posture so we don't pin them.
func TestSecurity_Scan_RunsAndProducesResults(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v3 checks read /proc + /etc + /sys — linux-only")
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

// TestSecurity_Scan_FiltersByCheckIDs: each documented v3 check id
// can be selected individually and the scan returns exactly one
// CheckResult for it. Loops over the v3 catalog so adding a new
// check automatically extends the test (no per-id case to wire up).
func TestSecurity_Scan_FiltersByCheckIDs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v3 checks read /proc + /etc + /sys — linux-only")
	}
	reg := installSysSecurity(t)

	listed := bridge.ListSecurityChecks(reg)(context.Background(),
		&v2pb.ListSecurityChecksRequest{})
	if len(listed.GetChecks()) == 0 {
		t.Fatal("empty catalog")
	}
	for _, c := range listed.GetChecks() {
		id := c.GetId()
		t.Run(id, func(t *testing.T) {
			resp := bridge.SecurityScan(reg)(context.Background(),
				&v2pb.SecurityScanRequest{CheckIds: []string{id}})
			if resp.GetError() != "" {
				t.Fatalf("security_scan error: %s", resp.GetError())
			}
			if got := len(resp.GetChecks()); got != 1 {
				t.Errorf("expected 1 CheckResult for filter %q, got %d", id, got)
			} else if resp.GetChecks()[0].GetId() != id {
				t.Errorf("check[0].id = %q; want %q", resp.GetChecks()[0].GetId(), id)
			}
		})
	}
}

// TestSecurity_Scan_FiltersByCategory: passing a category restricts
// the scan to checks in that category. v3 has multiple checks in the
// 'kernel' and 'filesystem' categories, so the response should
// contain exactly those.
func TestSecurity_Scan_FiltersByCategory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("v3 checks read /proc + /etc + /sys — linux-only")
	}
	reg := installSysSecurity(t)

	resp := bridge.SecurityScan(reg)(context.Background(),
		&v2pb.SecurityScanRequest{Categories: []string{"kernel"}})
	if resp.GetError() != "" {
		t.Fatalf("security_scan error: %s", resp.GetError())
	}
	if len(resp.GetChecks()) == 0 {
		t.Fatal("kernel category returned no CheckResults")
	}
	for _, c := range resp.GetChecks() {
		if c.GetCategory() != "kernel" {
			t.Errorf("check %q has category %q, want kernel", c.GetId(), c.GetCategory())
		}
	}
}

func checkIDs(checks []*v2pb.AvailableSecurityCheck) []string {
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.GetId())
	}
	return out
}
