package mesh

import (
	"sort"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// MeshNodeInfo is the topology-facing record for a single participant
// in the overlay. Combines self / direct-peer / LSDB-origin facts into
// one shape the REST layer can serialise directly.
type MeshNodeInfo struct {
	NodeID     string    `json:"node_id"`
	Self       bool      `json:"self"`
	Direct     bool      `json:"direct"`
	RemoteAddr string    `json:"remote_addr,omitempty"`
	Addresses  []string  `json:"addresses,omitempty"`
	LastSeen   time.Time `json:"last_seen,omitempty"`
}

// MeshEdgeInfo is an undirected link between two mesh nodes. NodeA and
// NodeB are sorted lexicographically so repeated observations from
// either side collapse to a single edge. Up is true iff the local node
// currently has a live direct link to one end — LSA-only edges (learnt
// from the LSDB but not locally observed) are reported with Up=false
// until the two peers corroborate the link.
type MeshEdgeInfo struct {
	NodeA    string        `json:"a"`
	NodeB    string        `json:"b"`
	Up       bool          `json:"up"`
	RTT      time.Duration `json:"rtt_ns,omitempty"`
	BytesIn  uint64        `json:"bytes_in"`
	BytesOut uint64        `json:"bytes_out"`
	MsgsIn   uint64        `json:"msgs_in"`
	MsgsOut  uint64        `json:"msgs_out"`
	// Since is the time the edge was first observed up. Zero when
	// edge is LSA-only (never locally adjacent).
	Since time.Time `json:"since,omitempty"`
}

// MeshTopology is the Node's view of the whole mesh at a point in time.
type MeshTopology struct {
	SelfNodeID  string         `json:"self_node_id"`
	Nodes       []MeshNodeInfo `json:"nodes"`
	Edges       []MeshEdgeInfo `json:"edges"`
	GeneratedAt time.Time      `json:"generated_at"`
}

// Topology returns a consistent snapshot of the mesh as seen from this
// node: every peer known through direct adjacency plus every LSA
// origin, and every undirected edge with direct-link telemetry
// attached. Safe for any goroutine.
func (n *Node) Topology() *MeshTopology {
	if n == nil {
		return &MeshTopology{GeneratedAt: time.Now()}
	}

	// 1) Collect direct links (have live stats) + their counters.
	direct := make(map[string]LinkStats, 8)
	for _, l := range n.linkSnapshot() {
		direct[l.PeerNodeID] = l.Stats()
	}

	// 2) Start the node set with self + every direct peer.
	nodeSet := map[string]*MeshNodeInfo{
		n.NodeID(): {NodeID: n.NodeID(), Self: true, Direct: false},
	}
	for peer, st := range direct {
		nodeSet[peer] = &MeshNodeInfo{
			NodeID:     peer,
			Direct:     true,
			RemoteAddr: st.RemoteAddr,
			LastSeen:   st.Since,
		}
	}

	// 3) LSDB origins and their declared links contribute both
	//    additional nodes and LSA-only edges.
	lsdbSnap := n.lsdb.Snapshot()
	edges := map[[2]string]*MeshEdgeInfo{}
	// LSDB.Snapshot returns a slice; rekey it by origin_node_id
	// for easier iteration below.
	lsdbByOrigin := make(map[string]*v2pb.MeshLSA, len(lsdbSnap))
	for _, lsa := range lsdbSnap {
		lsdbByOrigin[lsa.OriginNodeId] = lsa
	}

	// Helper: canonical pair ordering and upsert.
	upsertEdge := func(a, b string) *MeshEdgeInfo {
		if a > b {
			a, b = b, a
		}
		key := [2]string{a, b}
		if e, ok := edges[key]; ok {
			return e
		}
		e := &MeshEdgeInfo{NodeA: a, NodeB: b}
		edges[key] = e
		return e
	}

	for origin, lsa := range lsdbByOrigin {
		if _, ok := nodeSet[origin]; !ok {
			nodeSet[origin] = &MeshNodeInfo{NodeID: origin}
		}
		for _, lnk := range lsa.Links {
			if lnk.NodeId == "" {
				continue
			}
			if _, ok := nodeSet[lnk.NodeId]; !ok {
				nodeSet[lnk.NodeId] = &MeshNodeInfo{NodeID: lnk.NodeId}
			}
			upsertEdge(origin, lnk.NodeId)
		}
	}

	// 4) Overlay local direct-link facts onto matching edges (or
	//    create them if the LSA hasn't arrived yet).
	for peer, st := range direct {
		e := upsertEdge(n.NodeID(), peer)
		e.Up = true
		e.RTT = st.RTT
		e.BytesIn = st.BytesIn
		e.BytesOut = st.BytesOut
		e.MsgsIn = st.MsgsIn
		e.MsgsOut = st.MsgsOut
		e.Since = st.Since
	}

	// 5) Flatten with stable ordering.
	out := &MeshTopology{
		SelfNodeID:  n.NodeID(),
		GeneratedAt: time.Now(),
		Nodes:       make([]MeshNodeInfo, 0, len(nodeSet)),
		Edges:       make([]MeshEdgeInfo, 0, len(edges)),
	}
	for _, v := range nodeSet {
		out.Nodes = append(out.Nodes, *v)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].NodeID < out.Nodes[j].NodeID })
	for _, v := range edges {
		out.Edges = append(out.Edges, *v)
	}
	sort.Slice(out.Edges, func(i, j int) bool {
		if out.Edges[i].NodeA != out.Edges[j].NodeA {
			return out.Edges[i].NodeA < out.Edges[j].NodeA
		}
		return out.Edges[i].NodeB < out.Edges[j].NodeB
	})
	return out
}

// LinkObserver receives synchronous notifications when a mesh link
// transitions between up and down. Implementations must be cheap and
// non-blocking — they run on the link's management goroutine.
type LinkObserver interface {
	OnLinkUp(peerNodeID string, remoteAddr string)
	OnLinkDown(peerNodeID string)
}

// RegisterLinkObserver appends o to the observer list. Returns a
// func that removes it again; typical usage is to defer the
// unregister at the caller's lifecycle boundary.
func (n *Node) RegisterLinkObserver(o LinkObserver) func() {
	n.observerMu.Lock()
	n.observers = append(n.observers, o)
	n.observerMu.Unlock()
	return func() {
		n.observerMu.Lock()
		defer n.observerMu.Unlock()
		for i, existing := range n.observers {
			if existing == o {
				n.observers = append(n.observers[:i], n.observers[i+1:]...)
				return
			}
		}
	}
}

// notifyObservers invokes fn for every registered observer under a
// read lock.
func (n *Node) notifyObservers(fn func(LinkObserver)) {
	n.observerMu.RLock()
	defer n.observerMu.RUnlock()
	for _, o := range n.observers {
		fn(o)
	}
}
