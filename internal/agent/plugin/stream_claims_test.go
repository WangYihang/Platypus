package plugin_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestStream_NoClaimFallsThrough confirms the dispatcher returns
// (false, nil) for stream types no plugin claimed — letting the
// agent's built-in switch handle them. STREAM_TYPE_AGENT_UPGRADE is
// owned by the agent's own UpgradeHandler, never a plugin, so it's
// the canonical "no claim" case.
func TestStream_NoClaimFallsThrough(t *testing.T) {
	reg := freshRegistryWithSysPlugins(t)
	defer reg.Close(context.Background())

	handled, err := reg.DispatchStream(context.Background(),
		v2pb.StreamType_STREAM_TYPE_AGENT_UPGRADE, nil, nil)
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
	if handled {
		t.Errorf("expected handled=false for unclaimed type")
	}
}

// TestStream_PluginStreamRoutesToWasmDispatcher verifies that
// STREAM_TYPE_PLUGIN_STREAM bypasses the per-type claim lookup
// (which would never match — the type is the generic wasm-streaming
// slot, not a per-plugin claim) and routes straight into
// DispatchPluginStream. We feed garbage metadata; success here is
// observing handled=true with the wasm dispatcher's parse_metadata
// error code, which proves the route taken.
func TestStream_PluginStreamRoutesToWasmDispatcher(t *testing.T) {
	reg := freshRegistryWithSysPlugins(t)
	defer reg.Close(context.Background())

	a, b := net.Pipe()
	// Drain the wire side so the dispatcher's best-effort error frame
	// write doesn't deadlock against the synchronous net.Pipe.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for {
			var f v2pb.PluginStreamFrame
			if err := link.ReadFrame(b, &f); err != nil {
				return
			}
		}
	}()

	handled, err := reg.DispatchStream(context.Background(),
		v2pb.StreamType_STREAM_TYPE_PLUGIN_STREAM, a, []byte("not a proto"))

	// Close both ends + join the drain goroutine before any t.Fatal so
	// failure cases don't leak the read-blocked goroutine. If routing
	// worked, DispatchPluginStream already closed `a` via its defer
	// and these closes are no-ops; if it didn't, we close here.
	_ = a.Close()
	_ = b.Close()
	<-drainDone

	if !handled {
		t.Fatalf("expected handled=true (PLUGIN_STREAM owned by wasm dispatcher)")
	}
	if err == nil || !strings.Contains(err.Error(), "parse_metadata") {
		t.Errorf("err = %v, want parse_metadata", err)
	}
}

// freshRegistryWithSysPlugins is the local shared fixture: an empty
// Registry rooted at a temp dir. Both tests above only exercise the
// dispatcher's claim-table / type-routing logic, which doesn't need
// any plugins installed — the registry just has to exist.
func freshRegistryWithSysPlugins(t *testing.T) *plugin.Registry {
	t.Helper()
	root := t.TempDir()
	reg, err := plugin.New(plugin.Options{Paths: plugin.NewPaths(root)})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}
