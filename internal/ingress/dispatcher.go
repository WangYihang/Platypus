package ingress

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/log"
)

// ConnHandler receives a post-TLS net.Conn for its protocol. The
// handler owns the connection's lifetime from here on — the
// dispatcher does not close it.
type ConnHandler func(net.Conn)

// Config describes how the dispatcher is wired. TLSConfig must have
// Certificates and NextProtos set; use BuildTLSConfig to construct
// one with sensible defaults. HandshakeTimeout bounds how long a
// misbehaving client can hold a worker goroutine; zero means a
// 15-second default.
type Config struct {
	TLSConfig        *tls.Config
	HandshakeTimeout time.Duration
	HTTPBufferSize   int

	// OnAgent handles post-TLS connections that negotiated
	// ALPNAgent. Required if ALPNAgent appears in NextProtos.
	OnAgent ConnHandler
	// OnMesh handles connections that negotiated ALPNMesh.
	OnMesh ConnHandler
}

const (
	defaultHandshakeTimeout = 15 * time.Second
	defaultHTTPBufferSize   = 64
)

// Dispatcher is the single Accept loop for the unified TLS port. It
// performs the handshake on every accepted connection, reads the
// negotiated ALPN, and routes the bare net.Conn to the matching
// handler (agent / mesh) or pushes it onto a virtualListener for the
// gin HTTP server to pick up.
//
// A single Dispatcher owns one underlying net.Listener; start it with
// Serve(ctx) and stop it by cancelling the context or calling Close.
type Dispatcher struct {
	cfg Config
	vln *virtualListener

	closeOnce sync.Once
	closed    chan struct{}
}

// New builds a dispatcher from cfg. It does not start accepting
// connections — call Serve for that.
func New(cfg Config) (*Dispatcher, error) {
	if cfg.TLSConfig == nil {
		return nil, errors.New("ingress: TLSConfig required")
	}
	if len(cfg.TLSConfig.NextProtos) == 0 {
		return nil, errors.New("ingress: TLSConfig.NextProtos must list at least one ALPN")
	}
	if cfg.HandshakeTimeout <= 0 {
		cfg.HandshakeTimeout = defaultHandshakeTimeout
	}
	if cfg.HTTPBufferSize <= 0 {
		cfg.HTTPBufferSize = defaultHTTPBufferSize
	}
	return &Dispatcher{
		cfg:    cfg,
		closed: make(chan struct{}),
	}, nil
}

// HTTPListener returns the virtual net.Listener that yields every
// connection negotiating h2 or http/1.1. Hand this to
// http.Server.Serve. The virtual listener's Addr() mirrors the real
// underlying listener's address; callers don't need to know about
// the dispatcher's multiplexing.
//
// It is safe to call before Serve; the returned listener will block
// on Accept until a connection actually arrives.
func (d *Dispatcher) HTTPListener(addr net.Addr) net.Listener {
	if d.vln == nil {
		d.vln = newVirtualListener(addr, d.cfg.HTTPBufferSize)
	}
	return d.vln
}

// Serve takes ownership of the provided listener and accepts
// connections until ctx is cancelled or the listener returns a fatal
// error. It always closes ln before returning.
func (d *Dispatcher) Serve(ctx context.Context, ln net.Listener) error {
	if d.vln == nil {
		// Caller never asked for an HTTP listener — that's legal
		// (e.g. tests that only exercise the agent path), but we
		// still need the channel open so the dispatch loop doesn't
		// have to nil-check it.
		d.vln = newVirtualListener(ln.Addr(), d.cfg.HTTPBufferSize)
	}

	// Fire a goroutine that closes the underlying listener when
	// either Serve returns or ctx is cancelled.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		rawConn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			if isTemporaryNetError(err) {
				log.L.Warn("ingress_accept_transient", "error", err.Error())
				continue
			}
			return fmt.Errorf("ingress accept: %w", err)
		}
		go d.handshakeAndDispatch(rawConn)
	}
}

// Close shuts the HTTP virtual listener and stops accepting new
// connections. Already-dispatched connections are unaffected — they
// continue serving until their individual handlers close them.
func (d *Dispatcher) Close() error {
	d.closeOnce.Do(func() {
		close(d.closed)
		if d.vln != nil {
			_ = d.vln.Close()
		}
	})
	return nil
}

// handshakeAndDispatch runs on its own goroutine per accepted
// connection. Runs the TLS handshake under a timeout, reads
// NegotiatedProtocol, and dispatches. Any per-connection failure is
// logged at debug — a raw-port port scan shouldn't spam the error
// stream.
func (d *Dispatcher) handshakeAndDispatch(raw net.Conn) {
	tlsConn, ok := asTLS(raw, d.cfg.TLSConfig)
	if !ok {
		_ = raw.Close()
		return
	}

	if d.cfg.HandshakeTimeout > 0 {
		_ = tlsConn.SetDeadline(time.Now().Add(d.cfg.HandshakeTimeout))
	}
	if err := tlsConn.HandshakeContext(context.Background()); err != nil {
		log.L.Debug("ingress_handshake_failed",
			"remote", raw.RemoteAddr().String(),
			"error", err.Error(),
		)
		_ = tlsConn.Close()
		return
	}
	// Clear the handshake deadline — once we're dispatched it's the
	// handler's job to manage its own timeouts.
	_ = tlsConn.SetDeadline(time.Time{})

	proto := tlsConn.ConnectionState().NegotiatedProtocol
	switch proto {
	case ALPNAgent:
		if d.cfg.OnAgent == nil {
			log.L.Warn("ingress_no_agent_handler",
				"remote", raw.RemoteAddr().String())
			_ = tlsConn.Close()
			return
		}
		d.cfg.OnAgent(tlsConn)
	case ALPNMesh:
		if d.cfg.OnMesh == nil {
			log.L.Warn("ingress_no_mesh_handler",
				"remote", raw.RemoteAddr().String())
			_ = tlsConn.Close()
			return
		}
		d.cfg.OnMesh(tlsConn)
	case ALPNHTTP2, ALPNHTTP1, "":
		// Empty NegotiatedProtocol means the client didn't send ALPN
		// at all — Go's http.Transport suppresses it for HTTP/1-only
		// connections (e.g. WebSocket upgrades for coder/websocket),
		// and generic TLS tooling (openssl s_client without -alpn,
		// older curl versions) doesn't send it either. Treat it the
		// same as explicit http/1.1 and route into the HTTP listener
		// — previously these connections got silently closed, which
		// surfaced as unhelpful "EOF" errors on the client side and
		// took days to trace.
		if err := d.vln.push(tlsConn); err != nil {
			log.L.Debug("ingress_http_push_failed",
				"remote", raw.RemoteAddr().String(),
				"error", err.Error(),
			)
		}
	default:
		log.L.Debug("ingress_unknown_alpn",
			"remote", raw.RemoteAddr().String(),
			"negotiated", proto,
		)
		_ = tlsConn.Close()
	}
}

// asTLS promotes a raw connection to *tls.Conn. Accepts an
// already-wrapped connection (tests pre-wrap for ALPN control) or a
// plain net.Conn that needs tls.Server to drive the handshake.
func asTLS(c net.Conn, cfg *tls.Config) (*tls.Conn, bool) {
	if tc, ok := c.(*tls.Conn); ok {
		return tc, true
	}
	return tls.Server(c, cfg), true
}

// isTemporaryNetError checks whether an Accept error is recoverable.
// Mirrors the tried-and-true net/http pattern: back off on EAGAIN /
// too-many-open-files, bail on anything else.
func isTemporaryNetError(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		//nolint:staticcheck // net.Error.Temporary is the only portable accessor.
		if ne.Temporary() {
			return true
		}
	}
	return false
}
