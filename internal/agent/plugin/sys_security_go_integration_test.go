package plugin_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func installSysSecurityGo(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-security-go", "2.0.0", "sys_security.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-security-go", "2.0.0")

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
		PluginID:            "com.platypus.sys-security-go",
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

func TestSecurityGo_ListChecks(t *testing.T) {
	reg := installSysSecurityGo(t)

	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-security-go",
		Method:   "list_security_checks",
	})
	if resp.GetError() != "" {
		t.Fatalf("list_security_checks: %s", resp.GetError())
	}
	out := &v2pb.ListSecurityChecksResponse{}
	if err := protojson.Unmarshal(resp.GetPayload(), out); err != nil {
		t.Fatalf("unmarshal: %v\npayload: %s", err, resp.GetPayload())
	}
	got := map[string]bool{}
	for _, c := range out.GetChecks() {
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
			ids := make([]string, 0, len(out.GetChecks()))
			for _, c := range out.GetChecks() {
				ids = append(ids, c.GetId())
			}
			t.Errorf("v3 catalog missing check %q; got %v", id, ids)
		}
	}
}

func TestSecurityGo_Scan_RunsAndProducesResults(t *testing.T) {
	reg := installSysSecurityGo(t)

	body, _ := json.Marshal(struct{}{})
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-security-go",
		Method:   "security_scan",
		Payload:  body,
	})
	if resp.GetError() != "" {
		t.Fatalf("security_scan: %s", resp.GetError())
	}
	out := &v2pb.SecurityScanResponse{}
	if err := protojson.Unmarshal(resp.GetPayload(), out); err != nil {
		t.Fatalf("unmarshal: %v\npayload: %s", err, resp.GetPayload())
	}
	// Either of the two registered checks should have a CheckResult,
	// even if the host's kernel/sshd config produces zero findings.
	if len(out.GetChecks()) == 0 {
		t.Errorf("expected ≥1 CheckResult, got 0")
	}
}

func TestSecurityGo_Scan_FiltersByCheckIDs(t *testing.T) {
	reg := installSysSecurityGo(t)

	body, _ := json.Marshal(struct {
		CheckIDs []string `json:"check_ids"`
	}{CheckIDs: []string{"kernel.version"}})
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-security-go",
		Method:   "security_scan",
		Payload:  body,
	})
	if resp.GetError() != "" {
		t.Fatalf("security_scan: %s", resp.GetError())
	}
	out := &v2pb.SecurityScanResponse{}
	if err := protojson.Unmarshal(resp.GetPayload(), out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.GetChecks()) != 1 || out.GetChecks()[0].GetId() != "kernel.version" {
		t.Errorf("expected exactly check 'kernel.version'; got %+v", out.GetChecks())
	}
}
