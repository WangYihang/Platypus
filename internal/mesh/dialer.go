package mesh

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/protocol"
)

const (
	dialTimeout    = 8 * time.Second
	minRetryDelay  = 5 * time.Second
	maxRetryDelay  = 5 * time.Minute
	redialGracePct = 10 // ±10% jitter around the backoff timer
)

// Dialer owns all outbound connection attempts for a Node. Each known
// address is tracked in a small state machine: idle -> dialing -> linked
// or idle -> dialing -> backoff (on failure). Once a Link is established
// via an inbound or outbound attempt, Dialer stops trying alternate
// addresses for that peer until the link drops.
type Dialer struct {
	node   *Node
	logger *slog.Logger

	mu    sync.Mutex
	tasks map[string]*dialTask // key = "node_id|address"
}

type dialTask struct {
	nodeID  string
	addr    string
	backoff time.Duration
	stop    chan struct{}
}

func newDialer(node *Node) *Dialer {
	return &Dialer{
		node:   node,
		logger: node.logger,
		tasks:  map[string]*dialTask{},
	}
}

// EnsurePeer schedules (or adjusts) a dial task for each candidate
// address of the given peer. It's a no-op if we already have a live link
// to that peer.
func (d *Dialer) EnsurePeer(ctx context.Context, nodeID string, addresses []string) {
	if nodeID == "" || nodeID == d.node.identity.NodeID {
		return
	}
	if d.node.hasLink(nodeID) {
		return
	}
	for _, addr := range addresses {
		if addr == "" {
			continue
		}
		key := dialKey(nodeID, addr)
		d.mu.Lock()
		if _, ok := d.tasks[key]; ok {
			d.mu.Unlock()
			continue
		}
		task := &dialTask{
			nodeID:  nodeID,
			addr:    addr,
			backoff: minRetryDelay,
			stop:    make(chan struct{}),
		}
		d.tasks[key] = task
		d.mu.Unlock()

		go d.run(ctx, task)
	}
}

// StopPeer cancels any in-flight dial tasks for a peer (called when the
// peer went away or we already linked successfully).
func (d *Dialer) StopPeer(nodeID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, t := range d.tasks {
		if t.nodeID == nodeID {
			close(t.stop)
			delete(d.tasks, k)
		}
	}
}

func (d *Dialer) run(ctx context.Context, t *dialTask) {
	defer func() {
		d.mu.Lock()
		delete(d.tasks, dialKey(t.nodeID, t.addr))
		d.mu.Unlock()
	}()
	for {
		if ctx.Err() != nil {
			return
		}
		if d.node.hasLink(t.nodeID) {
			return
		}
		err := d.dialOnce(ctx, t)
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		delay := jittered(t.backoff)
		d.logger.Debug("mesh dial failed, backing off",
			slog.String("peer", t.nodeID),
			slog.String("addr", t.addr),
			slog.Duration("backoff", delay),
			slog.String("error", err.Error()))
		select {
		case <-ctx.Done():
			return
		case <-t.stop:
			return
		case <-time.After(delay):
		}
		t.backoff *= 2
		if t.backoff > maxRetryDelay {
			t.backoff = maxRetryDelay
		}
	}
}

func (d *Dialer) dialOnce(ctx context.Context, t *dialTask) error {
	tlsCfg, err := selfSignedTLSConfig()
	if err != nil {
		return err
	}
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", t.addr)
	if err != nil {
		return err
	}
	conn := tls.Client(raw, tlsCfg)
	if err := conn.HandshakeContext(dialCtx); err != nil {
		closeConn(raw)
		return fmt.Errorf("tls handshake: %w", err)
	}

	// Bounded deadline for the app-level mesh handshake.
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))

	codec := protocol.NewProtoCodec(conn)
	result, err := PerformClientHandshake(ctx, codec, d.node.identity, d.node.psk, d.node.advertisedAddrs())
	if err != nil {
		closeConn(conn)
		return err
	}
	if result.PeerNodeID == d.node.identity.NodeID {
		closeConn(conn)
		return fmt.Errorf("dialed self")
	}
	if t.nodeID != "" && result.PeerNodeID != t.nodeID {
		// The address advertised this NodeID but someone else answered.
		// We still completed a valid mesh handshake, so adopt the link
		// under the *real* NodeID — but don't short-circuit our attempts
		// for the original target.
		d.logger.Info("mesh dial: peer node_id mismatch",
			slog.String("want", t.nodeID),
			slog.String("got", result.PeerNodeID))
	}
	_ = conn.SetDeadline(time.Time{})

	link := newLink(conn, codec, result.PeerNodeID, result.PeerPublicKey, result.PeerAddresses, d.node)
	if !d.node.adoptLink(link) {
		// Already had a link to this peer (probably inbound beat us here).
		closeConn(conn)
		return nil
	}
	go link.run()
	return nil
}

func dialKey(nodeID, addr string) string {
	return nodeID + "|" + addr
}

// jittered adds ±(redialGracePct)% jitter to avoid thundering-herd
// reconnects when a shared upstream flaps.
func jittered(d time.Duration) time.Duration {
	if d <= 0 {
		return minRetryDelay
	}
	jitter := (int64(d) / 100) * int64(redialGracePct)
	if jitter <= 0 {
		return d
	}
	// Pseudo-random but cheap: derive from current nanos.
	offset := (time.Now().UnixNano() % (2 * jitter)) - jitter
	return d + time.Duration(offset)
}
