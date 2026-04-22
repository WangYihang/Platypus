package mesh

import (
	"crypto/ed25519"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/WangYihang/Platypus/internal/protocol"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

const (
	keepaliveInterval = 30 * time.Second
	keepaliveTimeout  = 90 * time.Second
)

// Link represents a live bidirectional mesh connection to a single peer.
// Exactly one Link exists per (local-node, peer-NodeID) pair; Node.linkMu
// enforces that. The Link owns the underlying net.Conn and the codec.
type Link struct {
	PeerNodeID    string
	PeerPublicKey ed25519.PublicKey
	RemoteAddr    string
	PeerAddresses []string

	conn  net.Conn
	codec *protocol.ProtoCodec
	node  *Node

	closeOnce sync.Once
	closed    chan struct{}
	lastRxNs  atomic.Int64
	logger    *slog.Logger
}

func newLink(
	conn net.Conn,
	codec *protocol.ProtoCodec,
	peer string,
	peerPub ed25519.PublicKey,
	peerAddrs []string,
	node *Node,
) *Link {
	l := &Link{
		PeerNodeID:    peer,
		PeerPublicKey: peerPub,
		RemoteAddr:    conn.RemoteAddr().String(),
		PeerAddresses: peerAddrs,
		conn:          conn,
		codec:         codec,
		node:          node,
		closed:        make(chan struct{}),
		logger:        node.logger.With(slog.String("peer", peer)),
	}
	l.lastRxNs.Store(time.Now().UnixNano())
	return l
}

// Send serialises an envelope onto the link. It is safe to call from
// multiple goroutines (ProtoCodec guards writes with a mutex).
func (l *Link) Send(env *agentpb.Envelope) error {
	return l.codec.Send(env)
}

// Close tears the link down. Idempotent.
func (l *Link) Close() {
	l.closeOnce.Do(func() {
		close(l.closed)
		_ = l.conn.Close()
	})
}

// Closed reports whether Close has been called.
func (l *Link) Closed() bool {
	select {
	case <-l.closed:
		return true
	default:
		return false
	}
}

// run executes the read loop + keepalive loop. It returns when the link
// is torn down. The caller is expected to have already finished the
// application-level handshake before invoking run.
func (l *Link) run() {
	defer l.Close()
	defer l.node.onLinkDown(l)

	l.node.onLinkUp(l)

	stopKeep := make(chan struct{})
	go l.keepaliveLoop(stopKeep)
	defer close(stopKeep)

	for {
		env, err := l.codec.Recv()
		if err != nil {
			if err != io.EOF && !l.Closed() {
				l.logger.Debug("mesh link recv error", slog.String("error", err.Error()))
			}
			return
		}
		l.lastRxNs.Store(time.Now().UnixNano())
		l.node.handleIncoming(l, env)
	}
}

func (l *Link) keepaliveLoop(stop chan struct{}) {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now().UnixNano()
			if now-l.lastRxNs.Load() > int64(keepaliveTimeout) {
				l.logger.Info("mesh link idle timeout, closing")
				l.Close()
				return
			}
			err := l.Send(&agentpb.Envelope{
				Version:   meshProtocolVersion,
				Timestamp: now,
				Payload: &agentpb.Envelope_MeshKeepalive{
					MeshKeepalive: &agentpb.MeshKeepalive{SentAt: now},
				},
			})
			if err != nil {
				l.logger.Debug("mesh keepalive send failed", slog.String("error", err.Error()))
				l.Close()
				return
			}
		}
	}
}
