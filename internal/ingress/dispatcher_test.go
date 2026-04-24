package ingress

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestDispatcherRoutesByALPN spins up a single dispatcher on an
// ephemeral port and fires three concurrent clients at it:
//
//   - one advertising ptps-agent
//   - one advertising ptps-mesh
//   - one advertising h2 / http/1.1 (a real http.Client, via a
//     vListener-backed http.Server)
//
// Each handler records the bytes it received; at the end we assert
// every handler saw exactly its own client's payload — which proves
// the dispatcher routed on ALPN rather than accidentally piping
// everything to the first registered callback.
func TestDispatcherRoutesByALPN(t *testing.T) {
	tlsCfg, err := BuildTLSConfig(CertSource{}, DefaultProtocols)
	if err != nil {
		t.Fatalf("BuildTLSConfig: %v", err)
	}

	agentPayload := []byte("hello-agent")
	meshPayload := []byte("hello-mesh")

	var (
		agentSeen []byte
		meshSeen  []byte
		hSeen     sync.Mutex
	)

	cfg := Config{
		TLSConfig:        tlsCfg,
		HandshakeTimeout: 5 * time.Second,
		OnAgent: func(c net.Conn) {
			defer c.Close()
			buf, _ := io.ReadAll(c)
			hSeen.Lock()
			agentSeen = buf
			hSeen.Unlock()
		},
		OnMesh: func(c net.Conn) {
			defer c.Close()
			buf, _ := io.ReadAll(c)
			hSeen.Lock()
			meshSeen = buf
			hSeen.Unlock()
		},
	}
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	httpListener := d.HTTPListener(ln.Addr())

	// Stand up a trivial http.Server on the virtual listener — this
	// verifies the HTTP path end-to-end: dispatcher ALPN match →
	// push onto vListener → http.Server.Serve pulls it and handles
	// the request.
	hsrv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hello-http"))
		}),
		ReadHeaderTimeout: 2 * time.Second,
	}
	go func() { _ = hsrv.Serve(httpListener) }()
	defer func() {
		_ = hsrv.Shutdown(context.Background())
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = d.Serve(ctx, ln) }()

	addr := ln.Addr().String()

	// --- agent client ---
	agentConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ALPNAgent},
	})
	if err != nil {
		t.Fatalf("agent dial: %v", err)
	}
	if got := agentConn.ConnectionState().NegotiatedProtocol; got != ALPNAgent {
		t.Fatalf("agent ALPN = %q, want %q", got, ALPNAgent)
	}
	_, _ = agentConn.Write(agentPayload)
	_ = agentConn.Close()

	// --- mesh client ---
	meshConn, err := tls.Dial("tcp", addr, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{ALPNMesh},
	})
	if err != nil {
		t.Fatalf("mesh dial: %v", err)
	}
	if got := meshConn.ConnectionState().NegotiatedProtocol; got != ALPNMesh {
		t.Fatalf("mesh ALPN = %q, want %q", got, ALPNMesh)
	}
	_, _ = meshConn.Write(meshPayload)
	_ = meshConn.Close()

	// --- http client ---
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				NextProtos:         []string{ALPNHTTP1},
			},
		},
		Timeout: 5 * time.Second,
	}
	resp, err := httpClient.Get("https://" + addr + "/")
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if got := string(body); got != "hello-http" {
		t.Fatalf("http body = %q, want hello-http", got)
	}

	// Wait for the agent + mesh handlers to record their payloads.
	// They run on dispatcher goroutines; ReadAll returns when the
	// other side closes, which happens synchronously above.
	deadline := time.Now().Add(2 * time.Second)
	for {
		hSeen.Lock()
		done := agentSeen != nil && meshSeen != nil
		hSeen.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("handlers never recorded payloads (agent=%q mesh=%q)",
				string(agentSeen), string(meshSeen))
		}
		time.Sleep(20 * time.Millisecond)
	}

	if string(agentSeen) != string(agentPayload) {
		t.Fatalf("agent handler got %q, want %q", agentSeen, agentPayload)
	}
	if string(meshSeen) != string(meshPayload) {
		t.Fatalf("mesh handler got %q, want %q", meshSeen, meshPayload)
	}
}

// TestDispatcherClosesUnknownALPN verifies that a client advertising
// an unrecognised ALPN gets its connection dropped rather than
// silently falling through to one of the handlers. This pins the
// "no compat window" decision.
func TestDispatcherClosesUnknownALPN(t *testing.T) {
	tlsCfg, err := BuildTLSConfig(CertSource{}, DefaultProtocols)
	if err != nil {
		t.Fatalf("BuildTLSConfig: %v", err)
	}
	called := false
	cfg := Config{
		TLSConfig: tlsCfg,
		OnAgent:   func(c net.Conn) { called = true; c.Close() },
		OnMesh:    func(c net.Conn) { called = true; c.Close() },
	}
	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = d.Serve(ctx, ln) }()

	// Client advertises an ALPN the server doesn't know about. TLS
	// spec says the server should either pick one of the overlap or
	// NOT advertise anything back; Go's crypto/tls returns "" when
	// there's no match (alpn fallback disabled). Dispatcher must
	// close that connection.
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"bogus-protocol"},
	})
	if err == nil {
		// Server may or may not reject the handshake outright —
		// newer Go versions send a no_application_protocol alert
		// when NextProtos has no overlap. Either way, the client
		// shouldn't be able to talk to a handler.
		defer conn.Close()
		// Send something; the dispatcher should have closed us.
		_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
		_, readErr := conn.Read(make([]byte, 1))
		if readErr == nil {
			t.Fatal("expected connection to be closed after unknown ALPN")
		}
	}
	// Whether the TLS handshake itself failed or the post-handshake
	// close fired, the handlers must never have been invoked.
	time.Sleep(100 * time.Millisecond)
	if called {
		t.Fatal("handler invoked for unknown ALPN")
	}
}

// TestVirtualListenerClose sanity-checks the adapter: Accept must
// return net.ErrClosed after Close, and Close is idempotent.
func TestVirtualListenerClose(t *testing.T) {
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
	vl := newVirtualListener(addr, 4)
	if err := vl.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := vl.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := vl.Accept(); err != net.ErrClosed {
		t.Fatalf("Accept after close = %v, want net.ErrClosed", err)
	}
}
