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

func installSysProcsGo(t *testing.T) *plugin.Registry {
	t.Helper()
	wasm := stagedWasmBytes(t, "com.platypus.sys-procs-linux-go", "1.0.0", "sys_procs.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-procs-linux-go", "1.0.0")

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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_procs.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close(context.Background()) })

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-procs-linux-go",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"fs.read"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return reg
}

// invokeGoProcessList mirrors the bridge's wire shape: input is the
// snake_case JSON the bridge.ProcessList encodes (NOT protojson, the
// plugin's request decoder uses serde defaults / encoding/json
// snake_case keys).  Output is protojson on the way back, same as
// the Rust plugin and what the bridge unmarshals.
func invokeGoProcessList(t *testing.T, reg *plugin.Registry, topN uint32, sortBy string) *v2pb.ProcessListResponse {
	t.Helper()
	body, err := json.Marshal(struct {
		TopN   uint32 `json:"top_n"`
		SortBy string `json:"sort_by"`
	}{TopN: topN, SortBy: sortBy})
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}
	resp := reg.Invoke(context.Background(), &v2pb.PluginCallRequest{
		PluginId: "com.platypus.sys-procs-linux-go",
		Method:   "process_list",
		Payload:  body,
	})
	if resp.GetError() != "" {
		t.Fatalf("process_list error: %s", resp.GetError())
	}
	out := &v2pb.ProcessListResponse{}
	if err := protojson.Unmarshal(resp.GetPayload(), out); err != nil {
		t.Fatalf("unmarshal: %v\npayload: %s", err, resp.GetPayload())
	}
	return out
}

func TestProcsGo_ReturnsCurrentProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc enumeration is linux-only")
	}
	reg := installSysProcsGo(t)
	resp := invokeGoProcessList(t, reg, 0, "")

	myPID := uint32(os.Getpid())
	for _, p := range resp.GetProcesses() {
		if p.GetPid() == myPID {
			return
		}
	}
	t.Errorf("self pid %d not in response (got %d processes)", myPID, len(resp.GetProcesses()))
}

func TestProcsGo_TopNCapsResultLength(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("/proc enumeration is linux-only")
	}
	reg := installSysProcsGo(t)
	resp := invokeGoProcessList(t, reg, 3, "")
	if got := len(resp.GetProcesses()); got > 3 {
		t.Errorf("top_n=3 returned %d processes; expected ≤ 3", got)
	}
	if resp.GetTotalCount() < uint32(len(resp.GetProcesses())) {
		t.Errorf("total_count %d < returned len %d", resp.GetTotalCount(), len(resp.GetProcesses()))
	}
}
