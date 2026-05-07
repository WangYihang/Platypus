package plugin_test

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func installSysProcsDarwin(t *testing.T) *plugin.Registry {
	t.Helper()
	const id = "com.platypus.sys-procs-darwin"
	const ver = "1.0.0"
	wasm := stagedWasmBytes(t, id, ver, "sys_procs_darwin.wasm")
	manifestBytes := stagedManifestBytes(t, id, ver)

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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_procs_darwin.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            id,
		Version:             ver,
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []plugin.CapabilityID{"exec"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestSysProcsDarwin_Manifest exercises the manifest plumbing: H1's
// reconciler-friendly os_targets, lang flag, the single RPC + the
// CapExec allowlist for `/bin/ps`.
func TestSysProcsDarwin_Manifest(t *testing.T) {
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-procs-darwin", "1.0.0")
	m, err := plugin.ParseManifest(manifestBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := m.Runtime.OSTargets; len(got) != 1 || got[0] != "darwin" {
		t.Errorf("os_targets = %v, want [darwin]", got)
	}
	if m.Runtime.Lang != "rust" {
		t.Errorf("lang = %q, want rust", m.Runtime.Lang)
	}
	if len(m.RPC) != 1 || m.RPC[0].Name != "process_list" {
		t.Errorf("rpc = %+v, want one entry named process_list", m.RPC)
	}
	if m.Capabilities.Exec == nil ||
		len(m.Capabilities.Exec.Commands) != 1 ||
		m.Capabilities.Exec.Commands[0] != "/bin/ps" {
		t.Errorf("exec capability mis-declared: %+v", m.Capabilities.Exec)
	}
}

// TestSysProcsDarwin_ProcessList_RoundTrip exercises the full plugin
// path. Skips on non-darwin since /bin/ps is BSD ps; the linux ps
// in this container has a different output schema and would report
// the wrong fields.
func TestSysProcsDarwin_ProcessList_RoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("sys-procs-darwin shells to BSD /bin/ps; runs on darwin only")
	}
	reg := installSysProcsDarwin(t)

	body, err := json.Marshal(struct {
		TopN   uint32 `json:"top_n"`
		SortBy string `json:"sort_by"`
	}{TopN: 10, SortBy: "rss"})
	if err != nil {
		t.Fatal(err)
	}
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-procs-darwin",
		Method:   "process_list",
		Payload:  body,
	})
	if resp.GetError() != "" {
		t.Fatalf("plugin call: %s", resp.GetError())
	}

	out := &v2pb.ProcessListResponse{}
	if err := protojson.Unmarshal(resp.GetPayload(), out); err != nil {
		t.Fatalf("unmarshal: %v\npayload: %s", err, resp.GetPayload())
	}
	if out.GetError() != "" {
		t.Fatalf("plugin error: %s", out.GetError())
	}
	if len(out.GetProcesses()) == 0 {
		t.Errorf("no processes returned; ps must have output something on a live darwin host")
	}
	if got := len(out.GetProcesses()); got > 10 {
		t.Errorf("len = %d, want <= top_n=10", got)
	}
	for i, p := range out.GetProcesses() {
		if p.GetPid() == 0 {
			t.Errorf("process[%d] has pid=0: %+v", i, p)
		}
		if p.GetName() == "" {
			t.Errorf("process[%d] has empty name: %+v", i, p)
		}
	}
}
