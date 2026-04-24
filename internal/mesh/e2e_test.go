package mesh

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// boot starts a Node on a random local port and stands up a minimal
// stand-in for the unified-ingress dispatcher: a TLS listener whose
// accept loop calls Node.AcceptRaw. Returns the node + its listen
// address.
func boot(t *testing.T, ctx context.Context, psk []byte, seeds []string) (*Node, string) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := Config{
		Identity:       mustIdentity(t),
		PSK:            psk,
		ListenAddr:     "", // ingress is managed by the test harness
		AdvertiseAddrs: nil,
		Peers:          seeds,
		Role:           "test",
	}
	n, err := NewNode(cfg, logger)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}

	tlsCfg, err := selfSignedTLSConfig()
	if err != nil {
		t.Fatalf("selfSignedTLSConfig: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	addr := ln.Addr().String()
	n.cfg.AdvertiseAddrs = []string{addr}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go n.AcceptRaw(ctx, conn)
		}
	}()

	if err := n.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return n, addr
}

var _ = (*net.TCPListener)(nil) // silence unused net import when tests shrink

// TestE2ETwoNodeHandshake brings up two in-process nodes with the same
// PSK and verifies they establish a mesh link.
func TestE2ETwoNodeHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	psk := randomPSK(t)

	a, _ := boot(t, ctx, psk, nil)
	_, bAddr := boot(t, ctx, psk, nil)
	// Wire A -> B.
	a.cfg.Peers = []string{bAddr}
	a.dialer.EnsurePeer(ctx, "bootstrap:"+bAddr, []string{bAddr})

	if !waitLink(a, 5*time.Second) {
		t.Fatal("A never formed any mesh link")
	}
}

// TestE2EThreeNodeRouting: A -- B -- C chain. A sends an envelope
// targeting C, must arrive at C (forwarded through B).
func TestE2EThreeNodeRouting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	psk := randomPSK(t)

	b, bAddr := boot(t, ctx, psk, nil)
	c, _ := boot(t, ctx, psk, nil)
	a, _ := boot(t, ctx, psk, nil)

	// Wire up: A dials B, C dials B. A has no direct link to C.
	a.dialer.EnsurePeer(ctx, "bootstrap:"+bAddr, []string{bAddr})
	c.dialer.EnsurePeer(ctx, "bootstrap:"+bAddr, []string{bAddr})

	// Wait for both links to come up.
	if !waitHasPeer(b, a.NodeID(), 5*time.Second) || !waitHasPeer(b, c.NodeID(), 5*time.Second) {
		t.Fatal("B didn't see both A and C as direct peers")
	}

	// Wait until A's routing table has a next-hop for C.
	if !waitRoute(a, c.NodeID(), 10*time.Second) {
		t.Fatalf("A never computed a route to C; routes=%v", a.routes.Snapshot())
	}

	// Install a payload handler on C so we can observe delivery.
	received := make(chan struct{}, 1)
	c.SetPayloadHandler(func(peer string, env *agentpb.Envelope) {
		// The peer that actually delivered may be B (forwarder), but the
		// envelope's SourceNode must be A.
		if env.SourceNode == a.NodeID() {
			select {
			case received <- struct{}{}:
			default:
			}
		}
	})

	// Send a ping envelope (any payload works — we reuse ExecRequest as
	// a harmless canary here).
	err := a.SendTo(c.NodeID(), &agentpb.Envelope{
		Payload: &agentpb.Envelope_ExecRequest{
			ExecRequest: &agentpb.ExecRequest{Command: "e2e-ping"},
		},
	})
	if err != nil {
		t.Fatalf("SendTo: %v", err)
	}

	select {
	case <-received:
		// Routed successfully. Intermediate hop B should have decremented TTL.
	case <-time.After(5 * time.Second):
		t.Fatalf("C did not receive envelope from A via B within 5s")
	}
}

func waitLink(n *Node, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(n.linkSnapshot()) > 0 {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func waitHasPeer(n *Node, peerID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if n.hasLink(peerID) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func waitRoute(n *Node, dst string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if n.routes.NextHop(dst) != "" {
			return true
		}
		// Force a recompute on every poll in case LSA was ingested but
		// no recompute was triggered (paranoid: the real code triggers
		// recompute already).
		n.recomputeRoutes()
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// discardWriter satisfies io.Writer for slog test silence.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

var _ = sync.Mutex{} // silence unused import if tests get stubbed
