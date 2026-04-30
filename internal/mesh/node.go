package mesh

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"

	"google.golang.org/protobuf/proto"
)

// PayloadHandler is invoked for every envelope addressed to the local
// NodeID. Upper layers plug their existing dispatcher in here; the
// mesh package itself knows nothing about process / tunnel / exec
// semantics.
type PayloadHandler func(peer string, env *v2pb.MeshEnvelope)

// Node is the top-level mesh participant. Each platypus-agent and
// platypus-server instance owns exactly one.
type Node struct {
	identity   *Identity
	trustedCAs *x509.CertPool
	cfg        Config
	logger     *slog.Logger

	dialer   *Dialer
	registry *Registry
	lsdb     *LSDB
	routes   *RouteTable

	linkMu         sync.RWMutex
	links          map[string]*Link  // NodeID -> live Link
	peerFloods     map[string]uint64 // origin -> highest MeshPeerDelta.seq seen
	lastLSASeq     uint64            // our own outbound LSA seq
	lastPeerDelta  atomic.Uint64
	payloadHandler atomic.Pointer[PayloadHandler]
	streams        *streamManager

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
//
// cfg.Identity (cert-bound, produced by LoadIdentityFromCert) and
// cfg.TrustedCAs (project CA pool) are required.
func NewNode(cfg Config, logger *slog.Logger) (*Node, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Identity == nil {
		return nil, fmt.Errorf("mesh: cfg.Identity is required (use LoadIdentityFromCert)")
	}
	if len(cfg.Identity.CertPEM) == 0 {
		return nil, fmt.Errorf("mesh: cfg.Identity must be cert-bound (CertPEM empty)")
	}
	if cfg.TrustedCAs == nil {
		return nil, fmt.Errorf("mesh: cfg.TrustedCAs is required")
	}
	n := &Node{
		identity:   cfg.Identity,
		trustedCAs: cfg.TrustedCAs,
		cfg:        cfg,
		logger: logger.With(
			slog.String("component", "mesh"),
			slog.String("node_id", cfg.Identity.NodeID),
		),
		registry:   newRegistry(),
		lsdb:       newLSDB(),
		routes:     newRouteTable(),
		links:      map[string]*Link{},
		peerFloods: map[string]uint64{},
		stopped:    make(chan struct{}),
	}
	n.streams = newStreamManager(n)
	if n.cfg.ProjectID == "" {
		n.cfg.ProjectID = "default"
	}
	// Seed the registry with ourselves so other nodes can learn our
	// public key when they ask for our peer list.
	n.registry.Upsert(&PeerRecord{
		NodeID:           n.identity.NodeID,
		PublicKey:        n.identity.PublicKey,
		Addresses:        n.advertisedAddrs(),
		LastSeen:         time.Now(),
		Role:             n.cfg.Role,
		BootstrapService: n.cfg.BootstrapEnabled,
		CertPEM:          append([]byte(nil), n.identity.CertPEM...),
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

// Start boots the dialer and periodic tasks. Inbound links arrive
// through AdoptStream — either via the server's
// /api/v1/mesh/link handler or, when --mesh-listen is set, an
// agent-side PeerListener mounting the same handler.
func (n *Node) Start(ctx context.Context) error {
	var startErr error
	n.startOnce.Do(func() {
		n.dialer = newDialer(n)
		// Seed dials for configured bootstrap peers. We don't know their
		// NodeIDs yet; use the address as a placeholder key. After a
		// successful handshake the real NodeID takes over.
		// currentPeers consults the Settings provider first, so a UI
		// edit made before Start() is visible here.
		for _, peer := range n.cfg.currentPeers() {
			if peer == "" {
				continue
			}
			n.dialer.EnsurePeer(ctx, "bootstrap:"+peer, []string{peer})
		}

		go n.lsaLoop(ctx)
		go n.registryLoop(ctx)
		go n.reconcileLoop(ctx)
		go n.runDiscovery(ctx)
	})
	return startErr
}

// SendTo routes env toward dst, filling in source_node/target_node/ttl.
// Returns an error if there's no path to dst.
func (n *Node) SendTo(dst string, env *v2pb.MeshEnvelope) error {
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

// ListenerAddr returns the ingress address the mesh node advertises.
// Pulled from cfg.AdvertiseAddrs when set, otherwise empty.
func (n *Node) ListenerAddr() string {
	if len(n.cfg.AdvertiseAddrs) > 0 {
		return n.cfg.AdvertiseAddrs[0]
	}
	return ""
}

// LocalNodeID returns this node's mesh identity (the URI SAN of its
// project-CA-signed cert).
func (n *Node) LocalNodeID() string { return n.identity.NodeID }

// AdoptStream registers a mesh peer link on a conn whose peer
// identity has already been validated (via mTLS at the transport
// layer). Blocks until the link tears down.
func (n *Node) AdoptStream(ctx context.Context, conn net.Conn, peerNodeID string, peerPubkey ed25519.PublicKey, peerCertPEM []byte) {
	if peerNodeID == "" || peerNodeID == n.identity.NodeID {
		closeConn(conn)
		return
	}
	codec := newEnvCodec(conn)
	link := newLink(conn, codec, peerNodeID, peerPubkey, nil, n)
	link.PeerCertPEM = append([]byte(nil), peerCertPEM...)
	if !n.adoptLink(link) {
		closeConn(conn)
		return
	}
	link.run()
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
	rec := n.registry.Get(l.PeerNodeID)
	if rec == nil {
		rec = &PeerRecord{NodeID: l.PeerNodeID, PublicKey: l.PeerPublicKey}
	}
	rec.PublicKey = l.PeerPublicKey
	rec.Addresses = mergeAddresses(rec.Addresses, l.PeerAddresses)
	if len(l.PeerCertPEM) > 0 {
		rec.CertPEM = append([]byte(nil), l.PeerCertPEM...)
	}
	rec.LastSeen = time.Now()
	n.registry.Upsert(rec)
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
	// known peers right away. Signed so the neighbour can still trust
	// the nodes list if the Announce gets relayed end-to-end instead
	// of staying link-local (future optimisation).
	announce := &v2pb.MeshPeerAnnounce{
		Nodes:         n.registry.ToNodeInfos(),
		OriginNodeId:  n.identity.NodeID,
		Pubkey:        n.identity.PublicKey,
		OriginCertPem: append([]byte(nil), n.identity.CertPEM...),
	}
	signPeerAnnounce(n.identity.PrivateKey, announce)
	_ = l.Send(&v2pb.MeshEnvelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload:   &v2pb.MeshEnvelope_PeerAnnounce{PeerAnnounce: announce},
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

func (n *Node) handleIncoming(from *Link, env *v2pb.MeshEnvelope) {
	if env.TargetNode != "" && env.TargetNode != n.identity.NodeID {
		n.forward(from, env)
		return
	}

	// Decide: mesh-control payload, locally-destined payload, or forward.
	switch p := env.Payload.(type) {
	case *v2pb.MeshEnvelope_Keepalive:
		// Updates the link's RTT from our last outbound keepalive
		// timestamp and captures the peer-reported lifetime
		// counters for cross-checking against the local codec.
		from.observeInboundKeepalive(p.Keepalive)
		return
	case *v2pb.MeshEnvelope_PeerAnnounce:
		n.ingestAnnounce(from, p.PeerAnnounce)
		return
	case *v2pb.MeshEnvelope_PeerDelta:
		n.ingestDelta(from, p.PeerDelta)
		return
	case *v2pb.MeshEnvelope_Lsa:
		n.ingestLSA(from, p.Lsa)
		return
	case *v2pb.MeshEnvelope_Unreachable:
		n.logger.Info("mesh unreachable notice",
			slog.String("target", p.Unreachable.TargetNode),
			slog.String("reason", p.Unreachable.Reason))
		return
	case *v2pb.MeshEnvelope_StreamOpen:
		n.streams.handleOpen(env)
		return
	case *v2pb.MeshEnvelope_StreamOpenAck:
		n.streams.handleOpenAck(env)
		return
	case *v2pb.MeshEnvelope_StreamData:
		n.streams.handleData(env)
		return
	case *v2pb.MeshEnvelope_StreamClose:
		n.streams.handleClose(env)
		return
	}

	if h := n.payloadHandler.Load(); h != nil && *h != nil {
		(*h)(from.PeerNodeID, env)
	} else {
		n.logger.Debug("envelope with no payload handler, dropped",
			slog.String("peer", from.PeerNodeID))
	}
}

func (n *Node) forward(from *Link, env *v2pb.MeshEnvelope) {
	if env.Ttl == 0 {
		return
	}
	env.Ttl--
	next := n.routes.NextHop(env.TargetNode)
	if next == "" {
		// Tell the originator there's no path.
		unreachable := &v2pb.MeshEnvelope{
			Version:    meshProtocolVersion,
			Timestamp:  time.Now().UnixNano(),
			SourceNode: n.identity.NodeID,
			TargetNode: env.SourceNode,
			Ttl:        maxEnvelopeTTL,
			Payload: &v2pb.MeshEnvelope_Unreachable{
				Unreachable: &v2pb.MeshUnreachable{
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

func (n *Node) ingestAnnounce(from *Link, ann *v2pb.MeshPeerAnnounce) {
	// Pre-v2 announces had no origin/sig fields; tolerate them as
	// unsigned from a trusted link-local peer to keep the migration
	// backward-compatible during rollout, but reject a populated
	// origin whose signature doesn't verify.
	if ann.GetOriginNodeId() != "" {
		if err := verifyPeerAnnounce(ann, n.trustedCAs); err != nil {
			n.logger.Warn("mesh peer announce sig rejected",
				slog.String("peer", from.PeerNodeID),
				slog.String("origin", ann.OriginNodeId),
				slog.String("error", err.Error()))
			return
		}
	}
	for _, ni := range ann.GetNodes() {
		if ni == nil || ni.NodeId == n.identity.NodeID {
			continue
		}
		rec := &PeerRecord{
			NodeID:           ni.NodeId,
			PublicKey:        ni.Pubkey,
			Addresses:        ni.Addresses,
			LastSeen:         time.Unix(ni.LastSeen, 0),
			Role:             ni.Role,
			BootstrapService: ni.BootstrapService,
			CertPEM:          append([]byte(nil), ni.CertPem...),
		}
		if n.registry.Upsert(rec) && n.dialer != nil {
			n.dialer.EnsurePeer(context.Background(), rec.NodeID, rec.Addresses)
		}
	}
}

func (n *Node) ingestDelta(from *Link, delta *v2pb.MeshPeerDelta) {
	if delta.OriginNodeId == "" {
		return
	}
	// Deltas flood; every hop MUST verify so a compromised
	// intermediate can't inject fake peer adds/removes.
	if err := verifyPeerDelta(delta, n.trustedCAs); err != nil {
		n.logger.Warn("mesh peer delta sig rejected",
			slog.String("peer", from.PeerNodeID),
			slog.String("origin", delta.OriginNodeId),
			slog.String("error", err.Error()))
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
			NodeID:           ni.NodeId,
			PublicKey:        ni.Pubkey,
			Addresses:        ni.Addresses,
			LastSeen:         time.Unix(ni.LastSeen, 0),
			Role:             ni.Role,
			BootstrapService: ni.BootstrapService,
			CertPEM:          append([]byte(nil), ni.CertPem...),
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
		relay := proto.Clone(delta).(*v2pb.MeshPeerDelta)
		relay.Ttl--
		out := &v2pb.MeshEnvelope{
			Version:   meshProtocolVersion,
			Timestamp: time.Now().UnixNano(),
			Payload:   &v2pb.MeshEnvelope_PeerDelta{PeerDelta: relay},
		}
		n.floodToAll(from, out)
	}
	_ = changed
}

func (n *Node) ingestLSA(from *Link, lsa *v2pb.MeshLSA) {
	changed, err := n.lsdb.Ingest(lsa, n.trustedCAs)
	if err != nil {
		n.logger.Debug("lsa rejected", slog.String("error", err.Error()))
		return
	}
	if !changed {
		return
	}
	n.recomputeRoutes()
	if lsa.FloodTtl > 1 {
		relay := proto.Clone(lsa).(*v2pb.MeshLSA)
		relay.FloodTtl--
		out := &v2pb.MeshEnvelope{
			Version:   meshProtocolVersion,
			Timestamp: time.Now().UnixNano(),
			Payload:   &v2pb.MeshEnvelope_Lsa{Lsa: relay},
		}
		n.floodToAll(from, out)
	}
}

// floodToAll sends env to every direct peer except except (which is
// usually the link the frame came in on, to avoid a trivial bounce-back).
func (n *Node) floodToAll(except *Link, env *v2pb.MeshEnvelope) {
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

	links := make([]*v2pb.MeshLSA_Link, 0, 4)
	for peer := range n.directPeerSet() {
		links = append(links, &v2pb.MeshLSA_Link{NodeId: peer, Cost: 1})
	}
	lsa := &v2pb.MeshLSA{
		OriginNodeId:  n.identity.NodeID,
		Seq:           seq,
		ExpiresAt:     time.Now().Add(lsaExpiry).Unix(),
		Links:         links,
		Pubkey:        n.identity.PublicKey,
		FloodTtl:      maxFloodTTL,
		OriginCertPem: append([]byte(nil), n.identity.CertPEM...),
	}
	// Sign over the canonical wire form without the sig or flood_ttl.
	canonCopy := proto.Clone(lsa).(*v2pb.MeshLSA)
	canonCopy.Sig = nil
	canonCopy.FloodTtl = 0
	canon, err := proto.Marshal(canonCopy)
	if err != nil {
		n.logger.Error("lsa marshal", slog.String("error", err.Error()))
		return
	}
	lsa.Sig = signBytes(n.identity.PrivateKey, canon)

	if _, err := n.lsdb.Ingest(lsa, n.trustedCAs); err != nil {
		n.logger.Error("self-lsa rejected", slog.String("error", err.Error()))
	}
	n.recomputeRoutes()
	env := &v2pb.MeshEnvelope{
		Version:   meshProtocolVersion,
		Timestamp: time.Now().UnixNano(),
		Payload:   &v2pb.MeshEnvelope_Lsa{Lsa: lsa},
	}
	n.floodToAll(nil, env)
}

func (n *Node) recomputeRoutes() {
	routes := computeRoutes(n.identity.NodeID, n.lsdb, n.directPeerSet())
	n.routes.Replace(routes)
}

func (n *Node) registryLoop(ctx context.Context) {
	sub := n.registry.Subscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-sub:
			if ev.NodeID == "" || ev.NodeID == n.identity.NodeID {
				continue
			}
			delta := &v2pb.MeshPeerDelta{
				OriginNodeId:  n.identity.NodeID,
				Seq:           n.lastPeerDelta.Add(1),
				Ttl:           maxFloodTTL,
				Pubkey:        n.identity.PublicKey,
				OriginCertPem: append([]byte(nil), n.identity.CertPEM...),
			}
			switch ev.Kind {
			case EventAdded, EventUpdated:
				if ev.Record == nil {
					continue
				}
				delta.Added = append(delta.Added, &v2pb.NodeInfo{
					NodeId:           ev.Record.NodeID,
					Pubkey:           ev.Record.PublicKey,
					Addresses:        append([]string(nil), ev.Record.Addresses...),
					LastSeen:         ev.Record.LastSeen.Unix(),
					Role:             ev.Record.Role,
					BootstrapService: ev.Record.BootstrapService,
					CertPem:          append([]byte(nil), ev.Record.CertPEM...),
				})
			case EventRemoved:
				delta.RemovedIds = append(delta.RemovedIds, ev.NodeID)
			default:
				continue
			}
			signPeerDelta(n.identity.PrivateKey, delta)
			n.floodToAll(nil, &v2pb.MeshEnvelope{
				Version:   meshProtocolVersion,
				Timestamp: time.Now().UnixNano(),
				Payload:   &v2pb.MeshEnvelope_PeerDelta{PeerDelta: delta},
			})
		}
	}
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
			// Bootstrap peers: re-read from the Settings provider on
			// every tick so admin add/remove takes effect without a
			// restart. EnsurePeer is idempotent; the dialer already
			// de-dups against live tasks.
			for _, peer := range n.cfg.currentPeers() {
				if peer == "" {
					continue
				}
				n.dialer.EnsurePeer(ctx, "bootstrap:"+peer, []string{peer})
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

// advertisedAddrs returns the addresses this node is willing to
// publish for other nodes to dial. The listening socket is owned by
// the platypus-server's HTTPS ingress (servers) or the
// PeerListener (agents with --mesh-listen).
func (n *Node) advertisedAddrs() []string {
	if len(n.cfg.AdvertiseAddrs) > 0 {
		return n.cfg.AdvertiseAddrs
	}
	if n.cfg.ListenAddr != "" {
		return []string{n.cfg.ListenAddr}
	}
	return nil
}

// ProjectID returns the project ID configured for this node.
func (n *Node) ProjectID() string { return n.cfg.ProjectID }

// IsServer returns true if this node is a Platypus Server.
func (n *Node) IsServer() bool { return n.cfg.Role == "server" }

// BootstrapEnabled reports whether this node accepts bootstrap streams.
func (n *Node) BootstrapEnabled() bool { return n.cfg.BootstrapEnabled }

// BootstrapTarget returns the local server endpoint used for bootstrap streams.
func (n *Node) BootstrapTarget() string { return n.cfg.BootstrapTarget }

// AdvertisedAddrs returns the list of addresses this node is publishing.
func (n *Node) AdvertisedAddrs() []string { return n.advertisedAddrs() }

// EnsurePeer schedules outbound dial attempts for a discovered peer.
func (n *Node) EnsurePeer(ctx context.Context, nodeID string, addresses []string) {
	if n == nil || n.dialer == nil || nodeID == "" || len(addresses) == 0 {
		return
	}
	n.dialer.EnsurePeer(ctx, nodeID, addresses)
}

// DialBootstrap opens a routed bootstrap stream to the target node.
func (n *Node) DialBootstrap(ctx context.Context, targetNodeID string) (net.Conn, error) {
	if n == nil || n.streams == nil {
		return nil, fmt.Errorf("mesh: stream manager not initialized")
	}
	return n.streams.DialBootstrap(ctx, targetNodeID)
}

// FindBootstrapServer returns a reachable server-capable node if one is known.
func (n *Node) FindBootstrapServer() (string, bool) {
	if n == nil {
		return "", false
	}
	for _, rec := range n.registry.Snapshot() {
		if rec.NodeID == "" || rec.NodeID == n.NodeID() {
			continue
		}
		if rec.Role != "server" || !rec.BootstrapService {
			continue
		}
		if n.hasLink(rec.NodeID) || n.routes.NextHop(rec.NodeID) != "" {
			return rec.NodeID, true
		}
	}
	return "", false
}
