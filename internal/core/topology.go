// Package core — Topology aggregator.
//
// BuildTopologySnapshot joins three independent views into a single
// shape the frontend's Cytoscape instance can render directly:
//
//  1. Mesh overlay (mesh.Node.Topology): the graph of mesh NodeIDs
//     and edges, with direct-link counters/RTT attached.
//  2. Runtime agent state (TCPServer.AgentClients): which agents are
//     currently connected, which machine they belong to, which
//     project they're scoped to.
//  3. Storage (storage.Host / Session): the canonical Host rows plus
//     historical sessions (including disconnected ones) so a machine
//     that was online yesterday still appears in the panel.
//
// Non-mesh deployments (Ctx.Mesh == nil) degrade gracefully: the
// snapshot synthesises a star topology with the server at the centre
// and every connected agent attached by a single synthetic edge.
// MeshEnabled=false lets the UI annotate accordingly.
package core

import (
	"context"
	"errors"
	"time"

	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/internal/storage"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// TopologyMachine is the compound "parent" node the UI renders: a
// physical / logical host with zero or more agent sessions attached.
// Sessions appear both as Active and Historical so operators can see
// ghost children for disconnected sessions.
type TopologyMachine struct {
	HostID      string            `json:"host_id"`
	ProjectID   string            `json:"project_id"`
	Hostname    string            `json:"hostname,omitempty"`
	MachineID   string            `json:"machine_id,omitempty"`
	OS          string            `json:"os,omitempty"`
	Fingerprint string            `json:"fingerprint"`
	FirstSeen   time.Time         `json:"first_seen_at"`
	LastSeen    time.Time         `json:"last_seen_at"`
	SysInfo     *agentpb.SysInfo  `json:"sys_info,omitempty"`
	Sessions    []TopologySession `json:"sessions"`
}

// TopologySession is one child entry on a machine compound node.
type TopologySession struct {
	ID             string     `json:"id"`
	Hash           string     `json:"hash,omitempty"`
	User           string     `json:"user,omitempty"`
	RemoteAddr     string     `json:"remote_addr,omitempty"`
	Version        string     `json:"version,omitempty"`
	ConnectedAt    time.Time  `json:"connected_at"`
	DisconnectedAt *time.Time `json:"disconnected_at,omitempty"`
	MeshNodeID     string     `json:"mesh_node_id,omitempty"` // empty for non-mesh agents
	Active         bool       `json:"active"`
}

// TopologyMeshNodeRef cross-references a mesh NodeID to its machine
// (when known). `Kind` is "self" for the server's own mesh node,
// "agent" for an enrolled machine, "unknown" when we only see it in
// the LSDB.
type TopologyMeshNodeRef struct {
	NodeID    string `json:"node_id"`
	Kind      string `json:"kind"`
	HostID    string `json:"host_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// TopologyLink is an edge in the snapshot. For mesh mode it maps
// 1:1 to mesh.MeshEdgeInfo. For star fallback it's server↔host links
// with per-agent session counters (currently zero; real counters land
// with the time-series writer in PR3).
type TopologyLink struct {
	A        string        `json:"a"` // mesh NodeID or "server"
	B        string        `json:"b"` // mesh NodeID or host_id
	Up       bool          `json:"up"`
	RTT      time.Duration `json:"rtt_ns,omitempty"`
	BytesIn  uint64        `json:"bytes_in"`
	BytesOut uint64        `json:"bytes_out"`
	MsgsIn   uint64        `json:"msgs_in"`
	MsgsOut  uint64        `json:"msgs_out"`
	Since    time.Time     `json:"since,omitempty"`
}

// TopologySnapshot is the wire shape the REST handler returns.
type TopologySnapshot struct {
	GeneratedAt time.Time             `json:"generated_at"`
	ProjectID   string                `json:"project_id"`
	MeshEnabled bool                  `json:"mesh_enabled"`
	Machines    []TopologyMachine     `json:"machines"`
	MeshNodes   []TopologyMeshNodeRef `json:"mesh_nodes"`
	Links       []TopologyLink        `json:"links"`
}

// BuildTopologySnapshot assembles the per-project topology view. It
// reads live runtime state + storage; callers must ensure the global
// Ctx is initialised (typical for HTTP handler dispatch).
func BuildTopologySnapshot(ctx context.Context, projectID string) (*TopologySnapshot, error) {
	if Ctx == nil {
		return nil, errors.New("core: context not initialised")
	}
	snap := &TopologySnapshot{
		GeneratedAt: time.Now().UTC(),
		ProjectID:   projectID,
		MeshEnabled: Ctx.Mesh != nil,
	}

	// --- 1. Host rows for the project. These seed the Machines list.
	//       Every Host becomes a compound node, even when no agent
	//       is currently connected (history stays visible).
	var hosts []*storage.Host
	if Ctx.Storage != nil {
		h, err := Ctx.Storage.Hosts().ListByProject(ctx, projectID)
		if err != nil {
			return nil, err
		}
		hosts = h
	}
	machineByID := make(map[string]*TopologyMachine, len(hosts))
	for _, h := range hosts {
		m := &TopologyMachine{
			HostID:      h.ID,
			ProjectID:   h.ProjectID,
			Hostname:    h.Hostname,
			MachineID:   h.MachineID,
			OS:          h.OS,
			Fingerprint: h.Fingerprint,
			FirstSeen:   h.FirstSeenAt,
			LastSeen:    h.LastSeenAt,
			Sessions:    []TopologySession{},
		}
		machineByID[h.ID] = m
	}

	// --- 2. Walk active agents: attach them as Active sessions and
	//       stamp the latest SysInfo on the parent machine.
	liveAgents := collectProjectAgents(projectID)
	for _, ac := range liveAgents {
		m, ok := machineByID[ac.HostID]
		if !ok {
			// Agent connected but no Host row yet (race or
			// pre-enrollment path). Synthesise a minimal machine
			// so the agent still shows up.
			m = &TopologyMachine{
				HostID:    ac.HostID,
				ProjectID: projectID,
				Hostname:  ac.Hostname,
				MachineID: ac.MachineID,
				OS:        ac.OS.String(),
				LastSeen:  ac.TimeStamp,
				FirstSeen: ac.TimeStamp,
				Sessions:  []TopologySession{},
			}
			machineByID[ac.HostID] = m
		}
		if e, ok := GetSysInfo(ac.Hash); ok {
			// Latest wins — if multiple agents run on the same
			// machine we overwrite on hash order, which is stable
			// enough for a best-effort display.
			m.SysInfo = e.Info
		}
		m.Sessions = append(m.Sessions, TopologySession{
			ID:          ac.Hash,
			Hash:        ac.Hash,
			User:        ac.User,
			RemoteAddr:  ac.OnelineDesc(),
			Version:     ac.Version,
			ConnectedAt: ac.TimeStamp,
			Active:      true,
		})
	}

	// --- 3. Historical (closed) sessions — one per row.
	if Ctx.Storage != nil {
		// Closed sessions: use ListForProject with Live filter off.
		closed := false
		rows, err := Ctx.Storage.Sessions().ListForProject(ctx, projectID, storage.SessionListOpts{Live: &closed, Limit: 500})
		if err == nil {
			for _, s := range rows {
				m, ok := machineByID[s.HostID]
				if !ok {
					continue
				}
				m.Sessions = append(m.Sessions, TopologySession{
					ID:             s.ID,
					User:           s.User,
					RemoteAddr:     s.RemoteAddr,
					Version:        s.Version,
					ConnectedAt:    s.ConnectedAt,
					DisconnectedAt: s.DisconnectedAt,
					Active:         false,
				})
			}
		}
	}

	for _, m := range machineByID {
		snap.Machines = append(snap.Machines, *m)
	}

	// --- 4. Mesh overlay — or star fallback.
	if node, ok := Ctx.Mesh.(*mesh.Node); ok && node != nil {
		mt := node.Topology()
		for _, n := range mt.Nodes {
			ref := TopologyMeshNodeRef{NodeID: n.NodeID}
			switch {
			case n.Self:
				ref.Kind = "self"
			case n.Direct:
				ref.Kind = "agent"
			default:
				ref.Kind = "unknown"
			}
			snap.MeshNodes = append(snap.MeshNodes, ref)
		}
		for _, e := range mt.Edges {
			snap.Links = append(snap.Links, TopologyLink{
				A: e.NodeA, B: e.NodeB,
				Up: e.Up, RTT: e.RTT,
				BytesIn: e.BytesIn, BytesOut: e.BytesOut,
				MsgsIn: e.MsgsIn, MsgsOut: e.MsgsOut,
				Since: e.Since,
			})
		}
	} else {
		// Star fallback: server node + one synthetic edge per
		// active agent. Counters are zero until PR3 tracks them
		// per-agent-session.
		snap.MeshNodes = append(snap.MeshNodes, TopologyMeshNodeRef{NodeID: "server", Kind: "self"})
		for _, ac := range liveAgents {
			snap.MeshNodes = append(snap.MeshNodes, TopologyMeshNodeRef{
				NodeID:    "host:" + ac.HostID,
				Kind:      "agent",
				HostID:    ac.HostID,
				ProjectID: projectID,
			})
			snap.Links = append(snap.Links, TopologyLink{
				A:     "server",
				B:     "host:" + ac.HostID,
				Up:    true,
				Since: ac.TimeStamp,
			})
		}
	}

	return snap, nil
}

// collectProjectAgents walks every live AgentClient and returns the
// subset whose ProjectID matches. The caller must tolerate duplicate
// HostIDs (multiple sessions per machine).
func collectProjectAgents(projectID string) []*AgentClient {
	var out []*AgentClient
	for _, ac := range allAgentClients() {
		if ac == nil || ac.ProjectID != projectID {
			continue
		}
		out = append(out, ac)
	}
	return out
}
