package plugin

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TDD for Registry.DispatchPluginStream — the entry point the agent
// dispatcher calls for STREAM_TYPE_PLUGIN_STREAM. Parses the
// PluginStreamRequest header, looks up the plugin + the matching
// streams[] manifest entry, validates the host_handler's wasm:
// marker, then runs runActiveStream + the two pumps on top of the
// wire stream.
//
// These tests cover the parse + lookup + validation error paths
// — i.e. the bits that don't need a real wasm plugin. The happy
// path (a real plugin echoes inbound bytes outbound through the
// pumps) lives in the integration test that ships alongside the
// first wasm-streaming plugin.

func TestDispatchPluginStream_BadMetadataReturnsParseError(t *testing.T) {
	reg := emptyRegistry(t)
	streamA, streamB := pipePair()
	defer streamA.Close()
	defer streamB.Close()
	drained := drainStream(t, streamB)
	defer func() { <-drained }()

	// Header metadata is supposed to be a marshalled
	// PluginStreamRequest; pass garbage instead.
	err := reg.DispatchPluginStream(context.Background(), streamA, []byte("not a proto"))
	if err == nil || !strings.Contains(err.Error(), "parse_metadata") {
		t.Errorf("err = %v, want parse_metadata", err)
	}
}

func TestDispatchPluginStream_UnknownPluginReturnsError(t *testing.T) {
	reg := emptyRegistry(t)
	streamA, streamB := pipePair()
	defer streamA.Close()
	defer streamB.Close()
	drained := drainStream(t, streamB)
	defer func() { <-drained }()

	meta := mustMarshal(t, &v2pb.PluginStreamRequest{
		PluginId:   "com.example.never-installed",
		StreamName: "echo",
	})
	err := reg.DispatchPluginStream(context.Background(), streamA, meta)
	if err == nil || !strings.Contains(err.Error(), "plugin_not_installed") {
		t.Errorf("err = %v, want plugin_not_installed", err)
	}
}

func TestDispatchPluginStream_DisabledPluginReturnsError(t *testing.T) {
	reg := emptyRegistry(t)
	injectPlugin(reg, &loaded{
		id: "com.example.echo",
		entry: CatalogEntry{
			ID:      "com.example.echo",
			Enabled: false,
		},
		manifest: &Manifest{
			Streams: []ManifestStream{
				{Name: "echo", StreamType: "STREAM_TYPE_PLUGIN_STREAM", HostHandler: "wasm:echo"},
			},
		},
	})

	streamA, streamB := pipePair()
	defer streamA.Close()
	defer streamB.Close()
	drained := drainStream(t, streamB)
	defer func() { <-drained }()
	meta := mustMarshal(t, &v2pb.PluginStreamRequest{
		PluginId: "com.example.echo", StreamName: "echo",
	})
	err := reg.DispatchPluginStream(context.Background(), streamA, meta)
	if err == nil || !strings.Contains(err.Error(), "plugin_disabled") {
		t.Errorf("err = %v, want plugin_disabled", err)
	}
}

func TestDispatchPluginStream_StreamNotDeclaredReturnsError(t *testing.T) {
	reg := emptyRegistry(t)
	injectPlugin(reg, &loaded{
		id: "com.example.echo",
		entry: CatalogEntry{
			ID: "com.example.echo", Enabled: true,
		},
		manifest: &Manifest{
			Streams: []ManifestStream{
				{Name: "other", StreamType: "STREAM_TYPE_PLUGIN_STREAM", HostHandler: "wasm:other"},
			},
		},
		pctx: &pluginCtx{},
	})

	streamA, streamB := pipePair()
	defer streamA.Close()
	defer streamB.Close()
	drained := drainStream(t, streamB)
	defer func() { <-drained }()
	meta := mustMarshal(t, &v2pb.PluginStreamRequest{
		PluginId: "com.example.echo", StreamName: "echo",
	})
	err := reg.DispatchPluginStream(context.Background(), streamA, meta)
	if err == nil || !strings.Contains(err.Error(), "stream_not_declared") {
		t.Errorf("err = %v, want stream_not_declared", err)
	}
}

func TestDispatchPluginStream_NonWasmHandlerReturnsError(t *testing.T) {
	// A plugin manifest that claims a stream with a host-provider
	// marker (host_handler="agent.X") cannot be served via the
	// PLUGIN_STREAM dispatcher — that path is wasm-only. The agent
	// would route it through Registry.DispatchStream's claim path
	// instead. If the server somehow opens PLUGIN_STREAM against
	// such a plugin we surface a clear error rather than silently
	// running the wrong handler.
	reg := emptyRegistry(t)
	injectPlugin(reg, &loaded{
		id: "com.example.legacy",
		entry: CatalogEntry{ID: "com.example.legacy", Enabled: true},
		manifest: &Manifest{
			Streams: []ManifestStream{
				{Name: "echo", StreamType: "STREAM_TYPE_PLUGIN_STREAM", HostHandler: "agent.legacy"},
			},
		},
		pctx: &pluginCtx{},
	})

	streamA, streamB := pipePair()
	defer streamA.Close()
	defer streamB.Close()
	drained := drainStream(t, streamB)
	defer func() { <-drained }()
	meta := mustMarshal(t, &v2pb.PluginStreamRequest{
		PluginId: "com.example.legacy", StreamName: "echo",
	})
	err := reg.DispatchPluginStream(context.Background(), streamA, meta)
	if err == nil || !strings.Contains(err.Error(), "non_wasm_handler") {
		t.Errorf("err = %v, want non_wasm_handler", err)
	}
}

// helpers ---------------------------------------------------------

// emptyRegistry constructs a Registry with no plugins + a Paths
// rooted at a temp dir. Catalog ops won't be exercised by these
// tests, but Registry's invariants want a real catalog handle.
func emptyRegistry(t *testing.T) *Registry {
	t.Helper()
	root := t.TempDir()
	reg, err := New(Options{Paths: NewPaths(root)})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return reg
}

// injectPlugin lets unit tests preload a Registry with a synthetic
// *loaded entry. Bypasses the install pipeline so the tests don't
// need real signed wasm.
func injectPlugin(r *Registry, l *loaded) {
	r.mu.Lock()
	defer r.mu.Unlock()
	emptyCorr := ""
	l.currentCorr.Store(&emptyCorr)
	if l.pctx == nil {
		l.pctx = &pluginCtx{id: l.id, manifest: l.manifest}
	}
	r.plugins[l.id] = l
}

// pipePair is a synchronous in-memory net.Conn pair like net.Pipe
// but with helpers in scope.
func pipePair() (net.Conn, net.Conn) {
	return net.Pipe()
}

// drainStream consumes frames written by DispatchPluginStream into
// the wire side of the pipe. net.Pipe is synchronous: the dispatcher's
// best-effort wireError write blocks unless someone is reading. In
// production the server is doing that read; tests stand in with this
// goroutine. Returns a done channel that closes after the read loop
// exits (peer closed or pipe broken) so the test can join cleanly.
func drainStream(t *testing.T, c net.Conn) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var frame v2pb.PluginStreamFrame
			if err := link.ReadFrame(c, &frame); err != nil {
				return
			}
		}
	}()
	return done
}

func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// --- belt-and-braces sanity: stub avoids unused-import warnings if
// the test set is later trimmed and time / atomic slip out of use.
var (
	_ = atomic.Bool{}
	_ = time.Second
)
