package mesh

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// PayloadHandler is invoked for every envelope whose target_node matches
// the local NodeID (i.e. addressed to us, not to be forwarded). The
// mesh package deliberately knows nothing about process/tunnel/exec
// semantics — upper layers plug their existing dispatcher in here.
type PayloadHandler func(peer string, env *agentpb.Envelope)

// Node is the top-level mesh participant. Each platypus-agent and
// platypus-server instance owns exactly one.
type Node struct {
	identity *Identity
	psk      []byte
	cfg      Config
	logger   *slog.Logger

	listener *Listener
	dialer   *Dialer
	registry *Registry
	lsdb     *LSDB
	routes   *RouteTable

	linkMu         sync.RWMutex
	links          map[string]*Link  // NodeID -> live Link
	peerFloods     map[string]uint64 // origin -> highest MeshPeerDelta.seq seen
	lastLSASeq     uint64            // our own outbound LSA seq
	payloadHandler atomic.Pointer[PayloadHandler]

	// Link event observers — called synchronously from onLinkUp /
	// onLinkDown. Used by higher layers (core.topology_events) to
	// fan out topology.* notify events without exposing internal
	// mesh state. Guarded by observerMu.
	observerMu sync.RWMutex
	observers  []LinkObserver

	startOnce sync.Once
	stopped   chan struct{}
}

// NewNode constructs and initialises a Node from cfg. It does NOT start
// any network activity — call Start for that.
func NewNode(cfg Config, logger *slog.Logger) (*Node, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Identity == nil {
		if cfg.IdentityDir == "" {
			cfg.IdentityDir = defaultIdentityDir(cfg.Role)
		}
		id, err := LoadOrCreateIdentity(cfg.IdentityDir)
		if err != nil {
			return nil, err
		}
		cfg.Identity = id
	}
	if len(cfg.PSK) == 0 {
		if cfg.PSKFile == "" {
			return nil, fmt.Errorf("mesh: PSK or PSKFile must be provided")
		}
		psk, err := LoadOrCreatePSK(cfg.PSKFile)
		if err != nil {
			return nil, err
		}
		cfg.PSK = psk
	}
	logger = logger.With(
		slog.String("component", "mesh"),
		slog.String("node_id", cfg.Identity.NodeID),
	)
	n := &Node{
		identity:   cfg.Identity,
		psk:        cfg.PSK,
		cfg:        cfg,
		logger:     logger,
		registry:   newRegistry(),
		lsdb:       newLSDB(),
		routes:     newRouteTable(),
		links:      map[string]*Link{},
		peerFloods: map[string]uint64{},
		stopped:    make(chan struct{}),
	}
	// Seed the registry with ourselves so other nodes can learn our
	// public key when they ask for our peer list.
	n.registry.Upsert(&PeerRecord{
		NodeID:    n.identity.NodeID,
		PublicKey: n.identity.PublicKey,
		Addresses: n.advertisedAddrs(),
		LastSeen:  time.Now(),
	})
	return n, nil
}

// Identity returns the node's identity (pubkey + NodeID).
func (n *Node) Identity() *Identity { return n.identity }

// NodeID is a convenience accessor.
func (n *Node) NodeID() string { return n.identity.NodeID }

// Registry exposes the known-peer table for UI / admin purposes.
func (n *Node) Registry() *Registry { return n.registry }

// SetPayloadHandler registers a callback to receive envelopes destined
// for the local node. Passing nil disables delivery (envelopes for the
// local node will be logged and dropped).
func (n *Node) SetPayloadHandler(h PayloadHandler) {
	if h == nil {
		n.payloadHandler.Store(nil)
		return
	}
	n.payloadHandler.Store(&h)
}

// Start boots the listener, dialer, and periodic tasks. Returns once
// the listener is ready (if one is configured). Blocks until ctx is
// cancelled in the background goroutines.
func (n *Node) Start(ctx context.Context) error {
	var startErr error
	n.startOnce.Do(func() {
		n.dialer = newDialer(n)
		if n.cfg.ListenAddr != "" {
			ln, err := newListener(n, n.cfg.ListenAddr)
			if err != nil {
				startErr = err
				return
			}
			n.listener = ln
			go func() {
				if err := ln.Serve(ctx); err != nil {
					n.logger.Error("mesh listener exited", slog.String("error", err.Error()))
				}
			}()
		}
		// Seed dials for configured bootstrap peers. We don't know their
		// NodeIDs yet; use the address as a placeholder key. After a
		// successful handshake the real NodeID takes over.
		for _, peer := range n.cfg.Peers {
			if peer == "" {
				continue
			}
			n.dialer.EnsurePeer(ctx, "bootstrap:"+peer, []string{peer})
		}

		go n.lsaLoop(ctx)
		go n.reconcileLoop(ctx)
	})
	return startErr
}

// SendTo routes env toward dst, filling in source_node/target_node/ttl.
// Returns an error if there's no path to dst.
func (n *Node) SendTo(dst string, env *agentpb.Envelope) error {
	if env == nil {
		return fmt.Errorf("mesh: nil envelope")
	}
	if dst == "" || dst == n.identity.NodeID {
		return fmt.Errorf("mesh: refusing to route to self")
	}
	env.SourceNode = n.identity.NodeID
	env.TargetNode = dst
	if env.Ttl == 0 {
		env.Ttl = maxEnvelopeTTL
	}
	next := n.routes.NextHop(dst)
	if next == "" {
		return fmt.Errorf("mesh: no route to %s", dst)
	}
	link := n.getLink(next)
	if link == nil {
		return fmt.Errorf("mesh: next hop %s has no link", next)
	}
	return link.Send(env)
}

// ListenerAddr returns the address the mesh listener is bound to, or
// "" if no listener is configured. Useful when ListenAddr is ":0".
func (n *Node) ListenerAddr() string {
	if n.listener == nil {
		return ""
	}
	return n.listener.Addr()
}

// ------------------------------------------------------------------
// Link lifecycle (called by listener.go and dialer.go)

// adoptLink registers a freshly handshaken link. Returns false if we
// already had a live link to that peer (duplicate inbound/outbound).
func (n *Node) adoptLink(l *Link) bool {
	n.linkMu.Lock()
	if existing, ok := n.links[l.PeerNodeID]; ok && !existing.Closed() {
		n.linkMu.Unlock()
		return false
	}
	n.links[l.PeerNodeID] = l
	n.linkMu.Unlock()

	// Record the peer + stop any pending dials to other addresses for them.
	n.registry.Upsert(&PeerRecord{
		NodeID:    l.PeerNodeID,
		PublicKey: l.PeerPublicKey,
		Addresses: l.PeerAddresses,
		LastSeen:  time.Now(),
	})
	if n.dialer != nil {
		n.dialer.StopPeer(l.PeerNodeID)
	}
	return true
}

func (n *Node) onLinkUp(l *Link) {
	n.logger.Info("mesh link up",
		slog.String("peer", l.PeerNodeID),
		slog.String("remote", l.RemoteAddr))
	n.notifyObservers(func(o LinkObserver) { o.OnLinkUp(l.PeerNodeID, l.RemoteAddr) })
	// Kick off a full announce to the new neighbour so it learns our
	// known peers right away.
	announce := &agentpb.MeshPeerAnnounce{Nodes: n.registry.ToNodeInfos()}
	_ = l.Send(&agentpb.Envelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload:   &agentpb.Envelope_MeshPeerAnnounce{MeshPeerAnnounce: announce},
	})
	// And re-broadcast our LSA to incorporate the new neighbour.
	n.publishLocalLSA()
}

func (n *Node) onLinkDown(l *Link) {
	n.linkMu.Lock()
	if cur := n.links[l.PeerNodeID]; cur == l {
		delete(n.links, l.PeerNodeID)
	}
	n.linkMu.Unlock()
	n.logger.Info("mesh link down", slog.String("peer", l.PeerNodeID))
	n.notifyObservers(func(o LinkObserver) { o.OnLinkDown(l.PeerNodeID) })
	// Refresh routes + LSA to reflect the change.
	n.publishLocalLSA()
	n.recomputeRoutes()

	// Try to reconnect if we still have addresses for them.
	if n.dialer != nil {
		if rec := n.registry.Get(l.PeerNodeID); rec != nil && len(rec.Addresses) > 0 {
			n.dialer.EnsurePeer(context.Background(), l.PeerNodeID, rec.Addresses)
		}
	}
}

func (n *Node) hasLink(peer string) bool {
	n.linkMu.RLock()
	defer n.linkMu.RUnlock()
	link, ok := n.links[peer]
	return ok && !link.Closed()
}

func (n *Node) getLink(peer string) *Link {
	n.linkMu.RLock()
	defer n.linkMu.RUnlock()
	return n.links[peer]
}

func (n *Node) linkSnapshot() []*Link {
	n.linkMu.RLock()
	defer n.linkMu.RUnlock()
	out := make([]*Link, 0, len(n.links))
	for _, l := range n.links {
		out = append(out, l)
	}
	return out
}

// LinkStats returns a snapshot of per-peer counter state for every
// currently established direct link. Used by the Topology aggregator
// to light up edge weights (bytes/s, msgs/s, RTT) on the Mesh
// visualisation.
func (n *Node) LinkStats() []LinkStats {
	links := n.linkSnapshot()
	out := make([]LinkStats, 0, len(links))
	for _, l := range links {
		out = append(out, l.Stats())
	}
	return out
}

func (n *Node) directPeerSet() map[string]struct{} {
	n.linkMu.RLock()
	defer n.linkMu.RUnlock()
	out := make(map[string]struct{}, len(n.links))
	for k, l := range n.links {
		if !l.Closed() {
			out[k] = struct{}{}
		}
	}
	return out
}

// ------------------------------------------------------------------
// Inbound envelope routing

func (n *Node) handleIncoming(from *Link, env *agentpb.Envelope) {
	// Decide: mesh-control payload, locally-destined payload, or forward.
	switch p := env.Payload.(type) {
	case *agentpb.Envelope_MeshKeepalive:
		// Updates the link's RTT from our last outbound keepalive
		// timestamp and captures the peer-reported lifetime
		// counters for cross-checking against the local codec.
		from.observeInboundKeepalive(p.MeshKeepalive)
		return
	case *agentpb.Envelope_MeshPeerAnnounce:
		n.ingestAnnounce(from, p.MeshPeerAnnounce)
		return
	case *agentpb.Envelope_MeshPeerDelta:
		n.ingestDelta(from, p.MeshPeerDelta)
		return
	case *agentpb.Envelope_MeshLsa:
		n.ingestLSA(from, p.MeshLsa)
		return
	case *agentpb.Envelope_MeshUnreachable:
		n.logger.Info("mesh unreachable notice",
			slog.String("target", p.MeshUnreachable.TargetNode),
			slog.String("reason", p.MeshUnreachable.Reason))
		return
	}

	if env.TargetNode != "" && env.TargetNode != n.identity.NodeID {
		n.forward(from, env)
		return
	}

	if h := n.payloadHandler.Load(); h != nil && *h != nil {
		(*h)(from.PeerNodeID, env)
	} else {
		n.logger.Debug("envelope with no payload handler, dropped",
			slog.String("peer", from.PeerNodeID))
	}
}

func (n *Node) forward(from *Link, env *agentpb.Envelope) {
	if env.Ttl == 0 {
		return
	}
	env.Ttl--
	next := n.routes.NextHop(env.TargetNode)
	if next == "" {
		// Tell the originator there's no path.
		unreachable := &agentpb.Envelope{
			Version:    meshProtocolVersion,
			Timestamp:  time.Now().UnixNano(),
			SourceNode: n.identity.NodeID,
			TargetNode: env.SourceNode,
			Ttl:        maxEnvelopeTTL,
			Payload: &agentpb.Envelope_MeshUnreachable{
				MeshUnreachable: &agentpb.MeshUnreachable{
					TargetNode: env.TargetNode,
					Reason:     "no route",
				},
			},
		}
		_ = from.Send(unreachable)
		return
	}
	link := n.getLink(next)
	if link == nil || link == from {
		// Routing-table inconsistency or direct loop — recompute and drop.
		n.recomputeRoutes()
		return
	}
	if err := link.Send(env); err != nil {
		n.logger.Debug("forward failed",
			slog.String("dst", env.TargetNode),
			slog.String("next", next),
			slog.String("error", err.Error()))
	}
}

// ------------------------------------------------------------------
// Gossip ingest

func (n *Node) ingestAnnounce(from *Link, ann *agentpb.MeshPeerAnnounce) {
	for _, ni := range ann.GetNodes() {
		if ni == nil || ni.NodeId == n.identity.NodeID {
			continue
		}
		rec := &PeerRecord{
			NodeID:    ni.NodeId,
			PublicKey: ni.Pubkey,
			Addresses: ni.Addresses,
			LastSeen:  time.Unix(ni.LastSeen, 0),
		}
		if n.registry.Upsert(rec) && n.dialer != nil {
			n.dialer.EnsurePeer(context.Background(), rec.NodeID, rec.Addresses)
		}
	}
}

func (n *Node) ingestDelta(from *Link, delta *agentpb.MeshPeerDelta) {
	if delta.OriginNodeId == "" {
		return
	}
	n.linkMu.Lock()
	last := n.peerFloods[delta.OriginNodeId]
	if delta.Seq <= last {
		n.linkMu.Unlock()
		return
	}
	n.peerFloods[delta.OriginNodeId] = delta.Seq
	n.linkMu.Unlock()

	changed := false
	for _, ni := range delta.GetAdded() {
		if ni == nil || ni.NodeId == n.identity.NodeID {
			continue
		}
		rec := &PeerRecord{
			NodeID:    ni.NodeId,
			PublicKey: ni.Pubkey,
			Addresses: ni.Addresses,
			LastSeen:  time.Unix(ni.LastSeen, 0),
		}
		if n.registry.Upsert(rec) {
			changed = true
			if n.dialer != nil {
				n.dialer.EnsurePeer(context.Background(), rec.NodeID, rec.Addresses)
			}
		}
	}
	for _, id := range delta.GetRemovedIds() {
		n.registry.Remove(id)
		changed = true
	}
	// Re-flood if this node wasn't the origin and we still have hops left.
	if delta.Ttl > 1 {
		relay := proto.Clone(delta).(*agentpb.MeshPeerDelta)
		relay.Ttl--
		out := &agentpb.Envelope{
			Version:   meshProtocolVersion,
			Timestamp: time.Now().UnixNano(),
			Payload:   &agentpb.Envelope_MeshPeerDelta{MeshPeerDelta: relay},
		}
		n.floodToAll(from, out)
	}
	_ = changed
}

func (n *Node) ingestLSA(from *Link, lsa *agentpb.MeshLSA) {
	changed, err := n.lsdb.Ingest(lsa)
	if err != nil {
		n.logger.Debug("lsa rejected", slog.String("error", err.Error()))
		return
	}
	if !changed {
		return
	}
	n.recomputeRoutes()
	if lsa.FloodTtl > 1 {
		relay := proto.Clone(lsa).(*agentpb.MeshLSA)
		relay.FloodTtl--
		out := &agentpb.Envelope{
			Version:   meshProtocolVersion,
			Timestamp: time.Now().UnixNano(),
			Payload:   &agentpb.Envelope_MeshLsa{MeshLsa: relay},
		}
		n.floodToAll(from, out)
	}
}

// floodToAll sends env to every direct peer except except (which is
// usually the link the frame came in on, to avoid a trivial bounce-back).
func (n *Node) floodToAll(except *Link, env *agentpb.Envelope) {
	for _, l := range n.linkSnapshot() {
		if l == except {
			continue
		}
		if err := l.Send(env); err != nil {
			n.logger.Debug("flood send failed",
				slog.String("peer", l.PeerNodeID),
				slog.String("error", err.Error()))
		}
	}
}

// ------------------------------------------------------------------
// LSA + routing

// publishLocalLSA builds a fresh LSA describing our directly connected
// neighbours, signs it, installs it in our own LSDB, and floods it.
func (n *Node) publishLocalLSA() {
	n.linkMu.Lock()
	n.lastLSASeq++
	seq := n.lastLSASeq
	n.linkMu.Unlock()

	links := make([]*agentpb.MeshLSA_Link, 0, 4)
	for peer := range n.directPeerSet() {
		links = append(links, &agentpb.MeshLSA_Link{NodeId: peer, Cost: 1})
	}
	lsa := &agentpb.MeshLSA{
		OriginNodeId: n.identity.NodeID,
		Seq:          seq,
		ExpiresAt:    time.Now().Add(lsaExpiry).Unix(),
		Links:        links,
		Pubkey:       n.identity.PublicKey,
		FloodTtl:     maxFloodTTL,
	}
	// Sign over the canonical wire form without the sig or flood_ttl.
	canonCopy := proto.Clone(lsa).(*agentpb.MeshLSA)
	canonCopy.Sig = nil
	canonCopy.FloodTtl = 0
	canon, err := proto.Marshal(canonCopy)
	if err != nil {
		n.logger.Error("lsa marshal", slog.String("error", err.Error()))
		return
	}
	lsa.Sig = signBytes(n.identity.PrivateKey, canon)

	if _, err := n.lsdb.Ingest(lsa); err != nil {
		n.logger.Error("self-lsa rejected", slog.String("error", err.Error()))
	}
	n.recomputeRoutes()
	env := &agentpb.Envelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload:   &agentpb.Envelope_MeshLsa{MeshLsa: lsa},
	}
	n.floodToAll(nil, env)
}

func (n *Node) recomputeRoutes() {
	routes := computeRoutes(n.identity.NodeID, n.lsdb, n.directPeerSet())
	n.routes.Replace(routes)
}

// ------------------------------------------------------------------
// Periodic tasks

func (n *Node) lsaLoop(ctx context.Context) {
	// Publish immediately so freshly-joined nodes have a signed LSA.
	n.publishLocalLSA()
	tick := time.NewTicker(2 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_ = n.lsdb.PurgeExpired(time.Now())
			n.publishLocalLSA()
		}
	}
}

func (n *Node) reconcileLoop(ctx context.Context) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if n.dialer == nil {
				continue
			}
			for _, rec := range n.registry.Snapshot() {
				if rec.NodeID == n.identity.NodeID {
					continue
				}
				if n.hasLink(rec.NodeID) {
					continue
				}
				n.dialer.EnsurePeer(ctx, rec.NodeID, rec.Addresses)
			}
		}
	}
}

// advertisedAddrs returns the addresses this node is willing to publish
// for other nodes to dial. If none are configured, it falls back to the
// listener's bound address.
func (n *Node) advertisedAddrs() []string {
	if len(n.cfg.AdvertiseAddrs) > 0 {
		return n.cfg.AdvertiseAddrs
	}
	if n.listener != nil {
		addr := n.listener.Addr()
		if addr != "" {
			return []string{addr}
		}
	}
	if n.cfg.ListenAddr != "" {
		return []string{n.cfg.ListenAddr}
	}
	return nil
}

// defaultIdentityDir picks a sensible default under the user's home dir.
// Falls back to ".platypus-mesh/<role>" in the cwd if HOME isn't set.
func defaultIdentityDir(role string) string {
	if role == "" {
		role = "node"
	}
	home, err := userHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".platypus-mesh", role)
	}
	return filepath.Join(home, ".platypus", "mesh", role)
}
