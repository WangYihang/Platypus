package mesh

import (
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
	"container/heap"
	"crypto/ed25519"
	"crypto/x509"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

)

const (
	// Maximum hops an envelope can traverse. Plenty of headroom even for
	// a densely partitioned mesh; prevents runaway loops if the routing
	// table is ever inconsistent.
	maxEnvelopeTTL = 32
	// Default flood scope for LSA / peer-delta broadcasts.
	maxFloodTTL = 16
	// Per-origin LSA TTL — new advertisements supersede anything older,
	// but after this we purge stale state for nodes that left quietly.
	lsaExpiry = 15 * time.Minute
)

// LSDB is the Link-State DataBase: one entry per origin, keyed by the
// origin's NodeID, carrying its latest advertisement. All methods are
// concurrency-safe.
type LSDB struct {
	mu    sync.RWMutex
	byOrg map[string]*v2pb.MeshLSA
}

func newLSDB() *LSDB {
	return &LSDB{byOrg: map[string]*v2pb.MeshLSA{}}
}

// Ingest stores lsa if its seq is strictly greater than the current
// value for its origin, and its signature verifies. Returns true if
// the LSDB changed.
//
// trustedCAs may be nil. When non-nil AND lsa.OriginCertPem is
// populated, the LSA's cert is chain-verified against the pool and
// the origin identity is read from the cert SAN (cert-bound mode).
// Otherwise verification falls back to the legacy
// DeriveNodeID(pubkey) == origin self-cert check — pure-mesh tests
// without a PKI continue to work.
func (db *LSDB) Ingest(lsa *v2pb.MeshLSA, trustedCAs *x509.CertPool) (bool, error) {
	if lsa == nil || lsa.OriginNodeId == "" {
		return false, fmt.Errorf("empty lsa")
	}
	if len(lsa.Pubkey) != ed25519.PublicKeySize {
		return false, fmt.Errorf("bad pubkey")
	}
	if err := verifyGossipOrigin("lsa", lsa.OriginCertPem, lsa.Pubkey, lsa.OriginNodeId, trustedCAs); err != nil {
		return false, err
	}
	sig := lsa.Sig
	// Verify against a copy with Sig blanked to a canonical empty value.
	lsaCopy := proto.Clone(lsa).(*v2pb.MeshLSA)
	lsaCopy.Sig = nil
	lsaCopy.FloodTtl = 0
	canon, err := proto.Marshal(lsaCopy)
	if err != nil {
		return false, err
	}
	if !ed25519.Verify(lsa.Pubkey, canon, sig) {
		return false, fmt.Errorf("bad signature")
	}

	db.mu.Lock()
	defer db.mu.Unlock()
	existing := db.byOrg[lsa.OriginNodeId]
	if existing != nil && existing.Seq >= lsa.Seq {
		return false, nil
	}
	db.byOrg[lsa.OriginNodeId] = proto.Clone(lsa).(*v2pb.MeshLSA)
	return true, nil
}

// PurgeExpired removes LSAs older than lsaExpiry based on expires_at or,
// failing that, wall-clock receipt time. Returns the list of origins
// purged so the node can emit Removed events if desired.
func (db *LSDB) PurgeExpired(now time.Time) []string {
	db.mu.Lock()
	defer db.mu.Unlock()
	var gone []string
	cutoff := now.Add(-lsaExpiry).Unix()
	for origin, lsa := range db.byOrg {
		expires := lsa.ExpiresAt
		if expires == 0 {
			continue
		}
		if expires < cutoff {
			delete(db.byOrg, origin)
			gone = append(gone, origin)
		}
	}
	return gone
}

// Snapshot returns a defensive copy of every LSA in the DB.
func (db *LSDB) Snapshot() []*v2pb.MeshLSA {
	db.mu.RLock()
	defer db.mu.RUnlock()
	out := make([]*v2pb.MeshLSA, 0, len(db.byOrg))
	for _, lsa := range db.byOrg {
		out = append(out, proto.Clone(lsa).(*v2pb.MeshLSA))
	}
	return out
}

// RouteTable maps destination NodeID -> next-hop NodeID.
type RouteTable struct {
	mu     sync.RWMutex
	routes map[string]string // dst -> nextHop
}

func newRouteTable() *RouteTable {
	return &RouteTable{routes: map[string]string{}}
}

// NextHop returns the next-hop NodeID for dst, or "" if unreachable.
func (rt *RouteTable) NextHop(dst string) string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.routes[dst]
}

// Replace swaps in a new routing table atomically.
func (rt *RouteTable) Replace(newRoutes map[string]string) {
	rt.mu.Lock()
	rt.routes = newRoutes
	rt.mu.Unlock()
}

// Snapshot returns a copy of the current routing table.
func (rt *RouteTable) Snapshot() map[string]string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	out := make(map[string]string, len(rt.routes))
	for k, v := range rt.routes {
		out[k] = v
	}
	return out
}

// computeRoutes runs Dijkstra over the LSDB + directly connected set,
// producing a next-hop table relative to selfID. Directly connected
// neighbours always route to themselves (they're the next hop). Nodes
// with no path are omitted.
func computeRoutes(selfID string, lsdb *LSDB, directPeers map[string]struct{}) map[string]string {
	graph := buildGraph(selfID, lsdb, directPeers)
	// Dijkstra from selfID.
	dist := map[string]uint32{selfID: 0}
	prev := map[string]string{}
	pq := &distHeap{}
	heap.Init(pq)
	heap.Push(pq, &distNode{id: selfID, d: 0})

	for pq.Len() > 0 {
		cur := heap.Pop(pq).(*distNode)
		if d, ok := dist[cur.id]; ok && cur.d > d {
			continue
		}
		for _, edge := range graph[cur.id] {
			alt := cur.d + edge.cost
			if d, ok := dist[edge.to]; !ok || alt < d {
				dist[edge.to] = alt
				prev[edge.to] = cur.id
				heap.Push(pq, &distNode{id: edge.to, d: alt})
			}
		}
	}

	routes := map[string]string{}
	for dst := range dist {
		if dst == selfID {
			continue
		}
		// Walk back from dst until predecessor is selfID; that predecessor's
		// successor (the node whose predecessor == self) is the next hop.
		cur := dst
		for prev[cur] != selfID {
			next, ok := prev[cur]
			if !ok {
				// Disconnected — should not happen because dist has it, but be safe.
				cur = ""
				break
			}
			cur = next
		}
		if cur == "" {
			continue
		}
		routes[dst] = cur
	}
	return routes
}

type edge struct {
	to   string
	cost uint32
}

func buildGraph(selfID string, lsdb *LSDB, directPeers map[string]struct{}) map[string][]edge {
	graph := map[string][]edge{}

	// Direct links — authoritative because we can see them ourselves.
	for peer := range directPeers {
		graph[selfID] = append(graph[selfID], edge{to: peer, cost: 1})
		graph[peer] = append(graph[peer], edge{to: selfID, cost: 1})
	}

	for _, lsa := range lsdb.Snapshot() {
		origin := lsa.OriginNodeId
		if origin == selfID {
			continue // our own advertised view is less trustworthy than our local state
		}
		for _, l := range lsa.Links {
			cost := l.Cost
			if cost == 0 {
				cost = 1
			}
			graph[origin] = append(graph[origin], edge{to: l.NodeId, cost: cost})
		}
	}
	return graph
}

// distNode and distHeap implement container/heap for Dijkstra.
type distNode struct {
	id string
	d  uint32
}

type distHeap []*distNode

func (h distHeap) Len() int            { return len(h) }
func (h distHeap) Less(i, j int) bool  { return h[i].d < h[j].d }
func (h distHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *distHeap) Push(x interface{}) { *h = append(*h, x.(*distNode)) }
func (h *distHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
