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

	// Time the link was promoted to up (run() entered the receive
	// loop). Used by Stats to annotate the counters' observation
	// window.
	sinceNs atomic.Int64

	// Timestamp of our most recently sent keepalive. The next
	// inbound keepalive from the peer is ~RTT old; we derive
	// lastRTTNs from that.
	lastOutboundKeepaliveNs atomic.Int64
	lastRTTNs               atomic.Int64

	// Peer-reported counters, last observed on an inbound
	// MeshKeepalive. Kept for cross-checking and debugging.
	peerBytesIn  atomic.Uint64
	peerBytesOut atomic.Uint64
	peerMsgsIn   atomic.Uint64
	peerMsgsOut  atomic.Uint64
}

// LinkStats is the observable counter snapshot for a live link. All
// fields are from the local perspective: BytesIn is what we actually
// received, BytesOut is what we actually wrote. The peer's
// cross-check view lives on the Link struct (peerBytes*, set from
// inbound keepalives).
type LinkStats struct {
	PeerNodeID string
	RemoteAddr string
	Since      time.Time     // when the link came up
	RTT        time.Duration // last measured round-trip
	BytesIn    uint64
	BytesOut   uint64
	MsgsIn     uint64
	MsgsOut    uint64
}

// Stats returns a best-effort point-in-time snapshot of this link's
// counters. Safe to call from any goroutine.
func (l *Link) Stats() LinkStats {
	return LinkStats{
		PeerNodeID: l.PeerNodeID,
		RemoteAddr: l.RemoteAddr,
		Since:      time.Unix(0, l.sinceNs.Load()),
		RTT:        time.Duration(l.lastRTTNs.Load()),
		BytesIn:    l.codec.BytesRecv(),
		BytesOut:   l.codec.BytesSent(),
		MsgsIn:     l.codec.MsgsRecv(),
		MsgsOut:    l.codec.MsgsSent(),
	}
}

// observeInboundKeepalive is called from Node.handleIncoming when a
// MeshKeepalive arrives. It updates RTT (if the peer echoed our last
// outbound keepalive's send time through in-flight timing) and
// captures the peer's reported lifetime counters.
func (l *Link) observeInboundKeepalive(ka *agentpb.MeshKeepalive) {
	if last := l.lastOutboundKeepaliveNs.Load(); last != 0 {
		rtt := time.Now().UnixNano() - last
		if rtt > 0 {
			l.lastRTTNs.Store(rtt)
		}
	}
	l.peerBytesIn.Store(ka.LifetimeBytesIn)
	l.peerBytesOut.Store(ka.LifetimeBytesOut)
	l.peerMsgsIn.Store(ka.LifetimeMsgsIn)
	l.peerMsgsOut.Store(ka.LifetimeMsgsOut)
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

	l.sinceNs.Store(time.Now().UnixNano())
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
			// Record this send BEFORE transmission so RTT
			// measurement on the reply is never negative even if
			// the peer ack'd it sub-microsecond.
			l.lastOutboundKeepaliveNs.Store(now)
			err := l.Send(&agentpb.Envelope{
				Version:   meshProtocolVersion,
				Timestamp: now,
				Payload: &agentpb.Envelope_MeshKeepalive{
					MeshKeepalive: &agentpb.MeshKeepalive{
						SentAt:           now,
						LifetimeBytesIn:  l.codec.BytesRecv(),
						LifetimeBytesOut: l.codec.BytesSent(),
						LifetimeMsgsIn:   l.codec.MsgsRecv(),
						LifetimeMsgsOut:  l.codec.MsgsSent(),
					},
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
