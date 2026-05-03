package plugin

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestTunnelPull_RustPluginRoundTrip drives sys-tunnel-pull through
// the legacy-wasm bridge against a localhost echo server. Asserts:
//   - TunnelPullResponse carries the resolved peer address
//   - bytes pushed to the wire reach the echo server and bounce back
func TestTunnelPull_RustPluginRoundTrip(t *testing.T) {
	wasmPath := tunnelPullWasmPath()
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("sys_tunnel_pull.wasm not built (%v) — run `cargo build --release --target wasm32-unknown-unknown` in example/plugins/sys-tunnel-pull/", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..",
		"example", "plugins", "sys-tunnel-pull", "plugin.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	// Spin up a tiny echo server on a free local port.
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
	paths := NewPaths(pluginRoot)
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PublisherKeyFile(HumanKeyID(pk)),
		[]byte(EncodePublicKey(pk, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestStr := strings.Replace(string(manifestBytes),
		"REPLACE_WITH_YOUR_KEY_ID", HumanKeyID(pk), 1)
	sig, err := Sign(sk, wasm, DefaultTrustedComment("sys_tunnel_pull.wasm"))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:            "com.platypus.sys-tunnel-pull",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"net.dial"},
	}, nil); err != nil {
		t.Fatal(err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.TunnelPullRequest{
		Target: echoTarget,
	})
	if err != nil {
		t.Fatal(err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := reg.DispatchStream(context.Background(),
			v2pb.StreamType_STREAM_TYPE_TUNNEL_PULL, streamA, meta)
		dispatchDone <- err
	}()

	// First frame: TunnelPullResponse with non-empty resolved_addr.
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

	// After the ack the wire is raw bytes — push some, expect them
	// back from the echo server.
	probe := []byte("ping over tunnel\n")
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

	// Closing streamB triggers the relay to unwind on its
	// wire->conn copy. The dispatch should return cleanly.
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

// TestTunnelPull_DeniesUnlistedTarget exercises the policy boundary:
// a manifest narrowed to a specific target must reject any other dial.
func TestTunnelPull_DeniesUnlistedTarget(t *testing.T) {
	wasmPath := tunnelPullWasmPath()
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Skipf("sys_tunnel_pull.wasm not built (%v)", err)
	}
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "..",
		"example", "plugins", "sys-tunnel-pull", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	pluginRoot := t.TempDir()
	paths := NewPaths(pluginRoot)
	sk, pk, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.PublishersDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.PublisherKeyFile(HumanKeyID(pk)),
		[]byte(EncodePublicKey(pk, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestStr := strings.Replace(string(manifestBytes),
		"REPLACE_WITH_YOUR_KEY_ID", HumanKeyID(pk), 1)
	// Narrow the wildcard to a specific allowed target.
	manifestStr = strings.Replace(manifestStr, `targets: ["*"]`,
		`targets: ["10.255.255.1:9999"]`, 1)
	sig, err := Sign(sk, wasm, DefaultTrustedComment("sys_tunnel_pull.wasm"))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := New(Options{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close(context.Background())

	if err := reg.InstallFromBytes(context.Background(), InstallParams{
		PluginID:            "com.platypus.sys-tunnel-pull",
		Version:             "1.0.0",
		PublisherPubkey:     []byte(EncodePublicKey(pk, "")),
		Manifest:            []byte(manifestStr),
		Wasm:                wasm,
		Signature:           []byte(EncodeSignature(sig)),
		Actor:               "test",
		GrantedCapabilities: []string{"net.dial"},
	}, nil); err != nil {
		t.Fatal(err)
	}

	streamA, streamB := net.Pipe()
	defer streamA.Close()
	defer streamB.Close()

	meta, err := proto.Marshal(&v2pb.TunnelPullRequest{
		Target: "127.0.0.1:1", // NOT in the narrowed allowlist
	})
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
	if !strings.Contains(strings.ToLower(ack.GetError()), "allowlist") {
		t.Errorf("error = %q, want to mention allowlist", ack.GetError())
	}

	select {
	case <-dispatchDone:
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchStream did not return")
	}
}

func tunnelPullWasmPath() string {
	return filepath.Join("..", "..", "..", "example", "plugins", "sys-tunnel-pull",
		"target", "wasm32-unknown-unknown", "release", "sys_tunnel_pull.wasm")
}
