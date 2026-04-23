package mesh

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/protocol"
	"github.com/WangYihang/Platypus/internal/utils/crypto"
)

// Listener accepts inbound mesh connections and drives them through the
// handshake before registering them with the owning Node.
type Listener struct {
	node   *Node
	addr   string
	tlsCfg *tls.Config
	ln     net.Listener
	logger *slog.Logger
}

// newListener binds the TCP+TLS listener synchronously so callers see a
// ready-to-use address immediately. The returned Listener is not yet
// accepting connections — call Serve in a goroutine to start the accept
// loop.
func newListener(node *Node, addr string) (*Listener, error) {
	tlsCfg, err := selfSignedTLSConfig()
	if err != nil {
		return nil, err
	}
	ln, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("mesh listener: listen: %w", err)
	}
	return &Listener{
		node:   node,
		addr:   addr,
		tlsCfg: tlsCfg,
		ln:     ln,
		logger: node.logger,
	}, nil
}

// Serve runs the accept loop. Blocks until ctx is cancelled or the
// listener is closed.
func (l *Listener) Serve(ctx context.Context) error {
	l.logger.Info("mesh listener started", slog.String("addr", l.ln.Addr().String()))
	go func() {
		<-ctx.Done()
		if l.ln != nil {
			_ = l.ln.Close()
		}
	}()
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("mesh listener: accept: %w", err)
		}
		go l.handleInbound(ctx, conn)
	}
}

func (l *Listener) handleInbound(ctx context.Context, conn net.Conn) {
	// Enforce a 5s deadline on the handshake phase; run() will disable it
	// once the link is up and keepalives take over.
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))

	codec := protocol.NewProtoCodec(conn)
	result, err := PerformServerHandshake(ctx, codec, l.node.identity, l.node.psk, l.node.advertisedAddrs())
	if err != nil {
		l.logger.Debug("mesh inbound handshake failed",
			slog.String("remote", conn.RemoteAddr().String()),
			slog.String("error", err.Error()))
		closeConn(conn)
		return
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		l.logger.Debug("clear deadline", slog.String("error", err.Error()))
	}

	link := newLink(conn, codec, result.PeerNodeID, result.PeerPublicKey, result.PeerAddresses, l.node)
	if !l.node.adoptLink(link) {
		link.logger.Info("duplicate mesh link, closing inbound")
		closeConn(conn)
		return
	}
	link.run()
}

// Addr returns the local address the listener is bound to. Safe to call
// from any goroutine because l.ln is set once, in newListener, before
// this Listener is ever shared.
func (l *Listener) Addr() string {
	if l.ln == nil {
		return l.addr
	}
	return l.ln.Addr().String()
}

// selfSignedTLSConfig builds a TLS config using the project's existing
// ephemeral self-signed cert helper. Mutual identity is proven at the
// application layer via the mesh handshake, so `InsecureSkipVerify` on
// the cert-chain level is acceptable here.
func selfSignedTLSConfig() (*tls.Config, error) {
	certBuilder := &strings.Builder{}
	keyBuilder := &strings.Builder{}
	crypto.Generate(certBuilder, keyBuilder)
	cert, err := tls.X509KeyPair([]byte(certBuilder.String()), []byte(keyBuilder.String()))
	if err != nil {
		return nil, fmt.Errorf("mesh tls config: %w", err)
	}
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	}, nil
}
