// Package core — Topology stream driver.
//
// StartTopologyStream runs a single background goroutine that:
//
//  1. Once per second, walks live mesh.Node.LinkStats() across all
//     projects that have at least one active agent and emits a
//     `topology.link_stats` event per project containing every edge.
//  2. On the same tick, walks the sysinfo cache and emits
//     `topology.machine_stats` events for any machine whose CPU /
//     memory sample changed since the previous tick.
//  3. Persists a row per edge and per host into the mesh_link_stats /
//     machine_stats time-series tables. Writes are best-effort — a DB
//     error logs but never blocks the broadcaster.
//  4. Runs a once-a-minute GC that deletes rows older than the
//     configured retention window (default 7 days).
//
// link_up / link_down are NOT emitted here — those fire synchronously
// from the mesh Node hooks (see internal/core/topology_events.go).
package core

import (
	"context"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/mesh"
	"github.com/WangYihang/Platypus/internal/storage"
)

const (
	// topologyTickInterval is how often the coalescer flushes stats.
	// Chosen to match the 1 Hz cadence UI edge-width animations look
	// natural at without flooding the WebSocket on a 50-link mesh.
	topologyTickInterval = 1 * time.Second

	// topologyRetention is how long time-series rows are kept.
	// Configurable later, hard-coded for now to keep the PR small.
	topologyRetention = 7 * 24 * time.Hour

	// topologyGCInterval is how often GC runs. Much larger than the
	// tick — deletes are cheap but we don't need them every second.
	topologyGCInterval = 1 * time.Minute
)

// StartTopologyStream kicks off the background goroutine. It returns
// a cancel func that stops the loop cleanly; the caller (typically
// main) defers it on shutdown.
func StartTopologyStream() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go runTopologyStream(ctx)
	return cancel
}

func runTopologyStream(ctx context.Context) {
	tick := time.NewTicker(topologyTickInterval)
	defer tick.Stop()
	gc := time.NewTicker(topologyGCInterval)
	defer gc.Stop()

	// Last-seen sample, indexed by host_id, so we only emit
	// machine_stats events when a new SysInfo has actually landed.
	lastSampledAt := map[string]int64{}
	var lastMu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			emitTopologyTick(lastSampledAt, &lastMu)
		case <-gc.C:
			if Ctx == nil || Ctx.Storage == nil {
				continue
			}
			n, err := Ctx.Storage.MeshStats().GCOlderThan(
				context.Background(), time.Now().Add(-topologyRetention))
			if err != nil {
				log.Info("topology_stream gc error: %s", err)
			} else if n > 0 {
				log.Debug("topology_stream gc removed %d rows", n)
			}
		}
	}
}

// emitTopologyTick is the per-second work unit. Broken out so tests
// can drive it directly without setting up a ticker.
func emitTopologyTick(lastSampledAt map[string]int64, lastMu *sync.Mutex) {
	if Ctx == nil {
		return
	}

	now := time.Now().UTC()

	// --- Per-project: collect active agents → machines so we know
	//     which projects to scope the link_stats frame to, and so we
	//     can look up the project_id for each link stat.
	projects := map[string][]*AgentClient{} // projectID -> agents
	for _, ac := range allAgentClients() {
		if ac == nil || ac.ProjectID == "" {
			continue
		}
		projects[ac.ProjectID] = append(projects[ac.ProjectID], ac)
	}

	// --- Mesh link stats: broadcast once per project with the same
	//     edge payload (mesh is shared across projects; project
	//     scoping filters the UI's subscription). DB persistence
	//     uses the agent-attached project_id as the canonical owner.
	if node, ok := Ctx.Mesh.(*mesh.Node); ok && node != nil {
		stats := node.LinkStats()
		if len(stats) > 0 {
			selfID := node.NodeID()
			// Build the broadcast payload once.
			payloadLinks := make([]map[string]interface{}, 0, len(stats))
			for _, st := range stats {
				a, b := selfID, st.PeerNodeID
				if a > b {
					a, b = b, a
				}
				payloadLinks = append(payloadLinks, map[string]interface{}{
					"a":         a,
					"b":         b,
					"rtt_ns":    st.RTT.Nanoseconds(),
					"bytes_in":  st.BytesIn,
					"bytes_out": st.BytesOut,
					"msgs_in":   st.MsgsIn,
					"msgs_out":  st.MsgsOut,
				})
			}
			for projectID := range projects {
				BroadcastNotify(EventTopologyLinkStats, map[string]interface{}{
					"project_id": projectID,
					"tick_at":    now,
					"links":      payloadLinks,
				})
			}
			// DB: one row per (project, edge). Group by the
			// project_id of any agent currently connected on that
			// link's peer NodeID. Without a NodeID→project map
			// (PKI phase), we fan out to every project that has
			// any agent — simplest and consistent with the
			// broadcast semantics.
			if Ctx.Storage != nil {
				var rows []storage.MeshLinkStat
				for projectID := range projects {
					for _, st := range stats {
						a, b := selfID, st.PeerNodeID
						if a > b {
							a, b = b, a
						}
						var rttPtr *int64
						if r := st.RTT.Nanoseconds(); r > 0 {
							rttPtr = &r
						}
						rows = append(rows, storage.MeshLinkStat{
							At:        now,
							ProjectID: projectID,
							NodeA:     a,
							NodeB:     b,
							BytesIn:   int64(st.BytesIn),
							BytesOut:  int64(st.BytesOut),
							MsgsIn:    int64(st.MsgsIn),
							MsgsOut:   int64(st.MsgsOut),
							RTTNs:     rttPtr,
						})
					}
				}
				if err := Ctx.Storage.MeshStats().InsertLinkStats(context.Background(), rows); err != nil {
					log.Info("topology_stream link stats insert: %s", err)
				}
			}
		}
	}

	// --- Per-machine sysinfo: walk the cache, emit when the sample
	//     is newer than what we last broadcast for that host.
	var machineRows []storage.MachineStat
	for projectID, agents := range projects {
		for _, ac := range agents {
			entry, ok := GetSysInfo(ac.Hash)
			if !ok || entry.Info == nil {
				continue
			}
			sampledAt := entry.Info.SampledAtUnix
			lastMu.Lock()
			prev := lastSampledAt[ac.HostID]
			if sampledAt > prev {
				lastSampledAt[ac.HostID] = sampledAt
				lastMu.Unlock()
				BroadcastNotify(EventTopologyMachineStats, map[string]interface{}{
					"project_id":  projectID,
					"host_id":     ac.HostID,
					"cpu_percent": entry.Info.CpuPercent,
					"mem_percent": entry.Info.MemPercent,
					"sampled_at":  sampledAt,
					"sys_info":    entry.Info,
				})
				cpu := entry.Info.CpuPercent
				mem := entry.Info.MemPercent
				machineRows = append(machineRows, storage.MachineStat{
					At:         time.Unix(sampledAt, 0).UTC(),
					HostID:     ac.HostID,
					ProjectID:  projectID,
					CPUPercent: &cpu,
					MemPercent: &mem,
				})
			} else {
				lastMu.Unlock()
			}
		}
	}
	if Ctx.Storage != nil && len(machineRows) > 0 {
		if err := Ctx.Storage.MeshStats().InsertMachineStats(context.Background(), machineRows); err != nil {
			log.Info("topology_stream machine stats insert: %s", err)
		}
	}
}
