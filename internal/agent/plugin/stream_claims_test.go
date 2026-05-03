package plugin_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/system"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestStream_ClaimDispatch boots the system bundle (which includes
// sys-streams), registers a fake provider for one of its claimed
// names, and asserts the registry's DispatchStream routes to it.
func TestStream_ClaimDispatch(t *testing.T) {
	called := false
	plugin.SetStreamProvider("agent.process", func(_ context.Context, _ io.ReadWriteCloser, _ []byte) error {
		called = true
		return nil
	})
	t.Cleanup(plugin.ResetStreamProvidersForTest)

	reg := freshRegistryWithSysPlugins(t)
	defer reg.Close(context.Background())

	handled, err := reg.DispatchStream(context.Background(),
		v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN, nil, nil)
	if err != nil {
		t.Fatalf("dispatch err: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true (sys-streams should claim PROCESS_OPEN)")
	}
	if !called {
		t.Errorf("expected provider to be called")
	}
}

// TestStream_NoClaimFallsThrough confirms the dispatcher returns
// (false, nil) for stream types no plugin claimed — letting the
// agent's legacy switch handle them.
func TestStream_NoClaimFallsThrough(t *testing.T) {
	plugin.ResetStreamProvidersForTest()

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

// TestStream_MissingProviderErrors fails loudly when a claim references
// an unknown provider — defends against a deployment where the agent
// build forgot to register a name the bundled plugin expects.
func TestStream_MissingProviderErrors(t *testing.T) {
	// Don't register agent.process; sys-streams' claim should
	// surface the unknown-provider error.
	plugin.ResetStreamProvidersForTest()

	reg := freshRegistryWithSysPlugins(t)
	defer reg.Close(context.Background())

	handled, err := reg.DispatchStream(context.Background(),
		v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN, nil, nil)
	if !handled {
		t.Fatalf("expected handled=true (claim exists, provider missing)")
	}
	if err == nil {
		t.Errorf("expected unknown-provider error")
	}
}

// TestStream_ProviderErrorPropagates surfaces the provider's own
// returned error to the dispatcher.
func TestStream_ProviderErrorPropagates(t *testing.T) {
	wantErr := errors.New("synthetic provider failure")
	plugin.SetStreamProvider("agent.process", func(_ context.Context, _ io.ReadWriteCloser, _ []byte) error {
		return wantErr
	})
	t.Cleanup(plugin.ResetStreamProvidersForTest)

	reg := freshRegistryWithSysPlugins(t)
	defer reg.Close(context.Background())

	_, err := reg.DispatchStream(context.Background(),
		v2pb.StreamType_STREAM_TYPE_PROCESS_OPEN, nil, nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want wraps %v", err, wantErr)
	}
}

// freshRegistryWithSysPlugins is the local shared fixture mirroring
// bridge_test.go's helper (kept independent because the bridge tests
// live in a different package).
func freshRegistryWithSysPlugins(t *testing.T) *plugin.Registry {
	t.Helper()
	root := t.TempDir()
	reg, err := plugin.New(plugin.Options{Paths: plugin.NewPaths(root)})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	embFS, err := system.EmbeddedFS()
	if err != nil {
		t.Fatalf("EmbeddedFS: %v", err)
	}
	if r := system.EnsureInstalled(context.Background(), reg, embFS); len(r.Failed) > 0 {
		t.Fatalf("system bootstrap failures: %+v", r.Failed)
	}
	return reg
}
