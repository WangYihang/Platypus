package mesh

import (
	"crypto/ed25519"
	"sync"
	"time"

	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// PeerRecord is one entry in the known-peer table. A record is kept as
// long as any node in the mesh has told us about it recently; it does
// NOT imply we have a live link.
type PeerRecord struct {
	NodeID           string
	PublicKey        ed25519.PublicKey
	Addresses        []string
	LastSeen         time.Time
	Role             string
	BootstrapService bool
}

// Registry is an in-memory, concurrency-safe peer table. Every node runs
// one. Callers get an event whenever a peer is added / updated / removed.
type Registry struct {
	mu    sync.RWMutex
	peers map[string]*PeerRecord
	seq   uint64 // monotonic, incremented on every local mutation

	subMu sync.Mutex
	subs  []chan RegistryEvent
}

// RegistryEvent is emitted by the Registry to all subscribers.
type RegistryEvent struct {
	Kind   EventKind
	NodeID string
	Record *PeerRecord // nil for EventRemoved
}

type EventKind int

const (
	EventAdded EventKind = iota + 1
	EventUpdated
	EventRemoved
)

func newRegistry() *Registry {
	return &Registry{peers: map[string]*PeerRecord{}}
}

// Subscribe returns a channel that receives events. Buffered; if the
// subscriber can't keep up, events are dropped (never blocks Upsert).
func (r *Registry) Subscribe() <-chan RegistryEvent {
	ch := make(chan RegistryEvent, 32)
	r.subMu.Lock()
	r.subs = append(r.subs, ch)
	r.subMu.Unlock()
	return ch
}

// Snapshot returns a copy of every known peer. Useful for fresh
// MeshPeerAnnounce frames.
func (r *Registry) Snapshot() []*PeerRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*PeerRecord, 0, len(r.peers))
	for _, p := range r.peers {
		out = append(out, copyPeer(p))
	}
	return out
}

// Get returns a copy of the record for a single peer, or nil.
func (r *Registry) Get(nodeID string) *PeerRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.peers[nodeID]; ok {
		return copyPeer(p)
	}
	return nil
}

// Upsert inserts or updates a peer. Returns true if anything actually
// changed (caller can use this to decide whether to re-gossip).
func (r *Registry) Upsert(rec *PeerRecord) bool {
	if rec == nil || rec.NodeID == "" {
		return false
	}
	if DeriveNodeID(rec.PublicKey) != rec.NodeID {
		return false
	}

	r.mu.Lock()
	existing, had := r.peers[rec.NodeID]
	changed := false
	if !had {
		r.peers[rec.NodeID] = copyPeer(rec)
		r.seq++
		changed = true
	} else {
		merged := copyPeer(existing)
		merged.PublicKey = rec.PublicKey
		merged.Addresses = mergeAddresses(existing.Addresses, rec.Addresses)
		merged.Role = rec.Role
		merged.BootstrapService = rec.BootstrapService
		if rec.LastSeen.After(existing.LastSeen) {
			merged.LastSeen = rec.LastSeen
		}
		if !peerEqual(existing, merged) {
			r.peers[rec.NodeID] = merged
			r.seq++
			changed = true
		}
	}
	r.mu.Unlock()

	if !changed {
		return false
	}
	if had {
		r.emit(RegistryEvent{Kind: EventUpdated, NodeID: rec.NodeID, Record: r.Get(rec.NodeID)})
	} else {
		r.emit(RegistryEvent{Kind: EventAdded, NodeID: rec.NodeID, Record: r.Get(rec.NodeID)})
	}
	return true
}

// Remove drops a peer. Usually called when an explicit departure notice
// is flooded, not merely on link disconnect (links drop often and come
// back — the peer still exists).
func (r *Registry) Remove(nodeID string) {
	r.mu.Lock()
	_, had := r.peers[nodeID]
	if had {
		delete(r.peers, nodeID)
		r.seq++
	}
	r.mu.Unlock()
	if had {
		r.emit(RegistryEvent{Kind: EventRemoved, NodeID: nodeID})
	}
}

func (r *Registry) emit(ev RegistryEvent) {
	r.subMu.Lock()
	subs := make([]chan RegistryEvent, len(r.subs))
	copy(subs, r.subs)
	r.subMu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func copyPeer(p *PeerRecord) *PeerRecord {
	if p == nil {
		return nil
	}
	pub := make(ed25519.PublicKey, len(p.PublicKey))
	copy(pub, p.PublicKey)
	addrs := make([]string, len(p.Addresses))
	copy(addrs, p.Addresses)
	return &PeerRecord{
		NodeID:           p.NodeID,
		PublicKey:        pub,
		Addresses:        addrs,
		LastSeen:         p.LastSeen,
		Role:             p.Role,
		BootstrapService: p.BootstrapService,
	}
}

func mergeAddresses(old, new []string) []string {
	seen := make(map[string]struct{}, len(old)+len(new))
	out := make([]string, 0, len(old)+len(new))
	for _, a := range old {
		if a == "" {
			continue
		}
		if _, ok := seen[a]; !ok {
			seen[a] = struct{}{}
			out = append(out, a)
		}
	}
	for _, a := range new {
		if a == "" {
			continue
		}
		if _, ok := seen[a]; !ok {
			seen[a] = struct{}{}
			out = append(out, a)
		}
	}
	return out
}

func peerEqual(a, b *PeerRecord) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.NodeID != b.NodeID || a.Role != b.Role || a.BootstrapService != b.BootstrapService || len(a.Addresses) != len(b.Addresses) {
		return false
	}
	for i := range a.Addresses {
		if a.Addresses[i] != b.Addresses[i] {
			return false
		}
	}
	return true
}

// ToNodeInfos converts registry records to wire-format NodeInfos, using
// the given "now" as last_seen when a record has none.
func (r *Registry) ToNodeInfos() []*agentpb.NodeInfo {
	snap := r.Snapshot()
	out := make([]*agentpb.NodeInfo, 0, len(snap))
	for _, p := range snap {
		last := p.LastSeen.Unix()
		if last < 0 {
			last = 0
		}
		out = append(out, &agentpb.NodeInfo{
			NodeId:           p.NodeID,
			Pubkey:           p.PublicKey,
			Addresses:        append([]string(nil), p.Addresses...),
			LastSeen:         last,
			Role:             p.Role,
			BootstrapService: p.BootstrapService,
		})
	}
	return out
}
