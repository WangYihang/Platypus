package plugin_test

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestTunnelPullGo_RoundTrip drives the TinyGo sys-tunnel-pull-go
// plugin against a localhost echo server. Mirrors the Rust crate's
// integration test (TestTunnelPull_RustPluginRoundTrip) one-to-one;
// any divergence in the two language implementations would surface
// here as a wire-shape mismatch.
func TestTunnelPullGo_RoundTrip(t *testing.T) {
	wasm := stagedWasmBytes(t, "com.platypus.sys-tunnel-pull-go", "1.0.0", "sys_tunnel_pull.wasm")
	manifestBytes := stagedManifestBytes(t, "com.platypus.sys-tunnel-pull-go", "1.0.0")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	echoTarget := ln.Addr().String()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(conn, conn)
	}()

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
	sig, err := plugin.Sign(sk, wasm, plugin.DefaultTrustedComment("sys_tunnel_pull.wasm"))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := plugin.New(plugin.Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), plugin.InstallParams{
		PluginID:            "com.platypus.sys-tunnel-pull-go",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(plugin.EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(plugin.EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"net.dial"},
	}, nil); err != nil {
		t.Fatal(err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.TunnelPullRequest{Target: echoTarget})
	if err != nil {
		t.Fatal(err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_TUNNEL_PULL, streamA, meta)
		dispatchDone <- err
	}()

	var ack v2pb.TunnelPullResponse
	if err := link.ReadFrame(streamB, &ack); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if ack.GetError() != "" {
		t.Fatalf("ack error: %s", ack.GetError())
	}
	if !strings.Contains(ack.GetResolvedAddr(), "127.0.0.1") {
		t.Errorf("resolved_addr = %q, want to contain 127.0.0.1", ack.GetResolvedAddr())
	}

	probe := []byte("ping over go tunnel\n")
	if _, err := streamB.Write(probe); err != nil {
		t.Fatalf("wire write: %v", err)
	}
	got := make([]byte, len(probe))
	if err := streamB.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(streamB, got); err != nil {
		t.Fatalf("wire read: %v", err)
	}
	if string(got) != string(probe) {
		t.Errorf("echo got %q, want %q", got, probe)
	}
	streamB.Close()

	select {
	case err := <-dispatchDone:
		if err != nil {
			t.Errorf("dispatch err: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("DispatchStream did not return")
	}
}
