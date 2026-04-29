package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/ipinfo"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// TopologyHandler serves the per-project mesh + machine topology
// snapshot used by the Topology page. Live snapshots come from
// joining the persistent hosts table with the in-memory live-link
// registry (AgentLinkService); historical stats come from the
// time-series tables populated by core.StartTopologyStream.
type TopologyHandler struct {
	db    *storage.DB
	links *core.AgentLinkService
}

// NewTopologyHandler wires in the storage handle for history queries.
// db may be nil when the server is started without persistence; the
// history endpoints then degrade to empty responses.
func NewTopologyHandler(db *storage.DB) *TopologyHandler {
	return &TopologyHandler{db: db}
}

// WithAgentLinks attaches the live-link registry so Get can flag
// hosts whose agent currently has an open yamux session. Optional —
// snapshots without it still render the persistent state.
func (h *TopologyHandler) WithAgentLinks(svc *core.AgentLinkService) *TopologyHandler {
	h.links = svc
	return h
}

// topologySnapshot is the JSON payload the Fleet graph view consumes.
// Field naming mirrors the TypeScript TopologySnapshot exactly so a
// rename on either side surfaces at compile time.
type topologySnapshot struct {
	GeneratedAt string                  `json:"generated_at"`
	ProjectID   string                  `json:"project_id"`
	MeshEnabled bool                    `json:"mesh_enabled"`
	Machines    []topologyMachine       `json:"machines"`
	MeshNodes   []topologyMeshNodeRef   `json:"mesh_nodes"`
	Links       []topologyLink          `json:"links"`
}

type topologySysInfo struct {
	KernelVersion   string  `json:"kernel_version,omitempty"`
	OSDistribution  string  `json:"os_distribution,omitempty"`
	Platform        string  `json:"platform,omitempty"`
	PlatformVersion string  `json:"platform_version,omitempty"`
	CPUPercent      *float64 `json:"cpu_percent,omitempty"`
	MemPercent      *float64 `json:"mem_percent,omitempty"`
	MemTotalBytes   int64   `json:"mem_total_bytes,omitempty"`
	MemUsedBytes    int64   `json:"mem_used_bytes,omitempty"`
	UptimeSeconds   int64   `json:"uptime_seconds,omitempty"`
	SampledAtUnix   int64   `json:"sampled_at_unix,omitempty"`
}

type topologySession struct {
	ID             string       `json:"id"`
	Hash           string       `json:"hash,omitempty"`
	User           string       `json:"user,omitempty"`
	RemoteAddr     string       `json:"remote_addr,omitempty"`
	RemoteInfo     *ipinfo.Info `json:"remote_info,omitempty"`
	Version        string       `json:"version,omitempty"`
	ConnectedAt    string       `json:"connected_at"`
	DisconnectedAt string       `json:"disconnected_at,omitempty"`
	MeshNodeID     string       `json:"mesh_node_id,omitempty"`
	Active         bool         `json:"active"`
}

type topologyMachine struct {
	HostID       string            `json:"host_id"`
	ProjectID    string            `json:"project_id"`
	Hostname     string            `json:"hostname,omitempty"`
	MachineID    string            `json:"machine_id,omitempty"`
	OS           string            `json:"os,omitempty"`
	Fingerprint  string            `json:"fingerprint"`
	FirstSeenAt  string            `json:"first_seen_at"`
	LastSeenAt   string            `json:"last_seen_at"`
	SysInfo      *topologySysInfo  `json:"sys_info,omitempty"`
	Sessions     []topologySession `json:"sessions"`
}

type topologyMeshNodeRef struct {
	NodeID    string `json:"node_id"`
	Kind      string `json:"kind"` // "self" | "agent" | "unknown"
	HostID    string `json:"host_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type topologyLink struct {
	A        string `json:"a"`
	B        string `json:"b"`
	Up       bool   `json:"up"`
	RTTNs    int64  `json:"rtt_ns,omitempty"`
	BytesIn  int64  `json:"bytes_in"`
	BytesOut int64  `json:"bytes_out"`
	MsgsIn   int64  `json:"msgs_in"`
	MsgsOut  int64  `json:"msgs_out"`
	Since    string `json:"since,omitempty"`
}

// Get handles GET /api/v1/projects/:pid/topology.
//
// Snapshot strategy: the v2 mesh streamer that used to feed live
// link telemetry isn't fully ported yet, so we build a "best
// available" graph from the data we already have:
//
//   * one "self" mesh node for the server (always present so the
//     graph isn't empty before any agent enrolls);
//   * one machine per host row in the project, plus an "agent" mesh
//     node per host whose agent is currently registered in
//     AgentLinkService;
//   * each live agent gets a link from "self" to its mesh node, so
//     the graph at least shows reachability;
//   * sessions for each host pulled from the persistent sessions
//     table so the compound-parent layout still has its child
//     diamonds.
//
// Once mesh telemetry is back in v2 the link slice will start
// reporting RTT / bytes / msgs from the real link_stats events; the
// shape stays compatible.
//
// @Summary     Project topology snapshot (live + persistent)
// @Tags        topology
// @Produce     json
// @Param       pid path string true "Project ID"
// @Security    BearerAuth
// @Router      /api/v1/projects/{pid}/topology [get]
func (h *TopologyHandler) Get(c *gin.Context) {
	pid := c.Param("pid")
	now := time.Now().UTC()
	snap := topologySnapshot{
		GeneratedAt: now.Format(time.RFC3339Nano),
		ProjectID:   pid,
		MeshEnabled: false,
		Machines:    []topologyMachine{},
		MeshNodes:   []topologyMeshNodeRef{},
		Links:       []topologyLink{},
	}

	// 1. Server "self" node — always emitted so the graph view never
	//    renders empty for a project that's correctly running but
	//    has no enrolled hosts yet. Operators reading the graph
	//    expect at least the controller they're talking to to show
	//    up; a blank canvas reads like a broken UI.
	const serverNodeID = "server"
	snap.MeshNodes = append(snap.MeshNodes, topologyMeshNodeRef{
		NodeID:    serverNodeID,
		Kind:      "self",
		ProjectID: pid,
	})

	if h.db == nil {
		c.JSON(http.StatusOK, snap)
		return
	}

	// 2. Hosts → compound parents. Skipping persistence-less servers.
	hosts, err := h.db.Hosts().ListByProject(c.Request.Context(), pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list hosts"})
		return
	}

	// Bulk-fetch live sessions once instead of N times.
	liveSessions, _ := h.db.Sessions().ListLiveForProject(c.Request.Context(), pid)
	liveByHost := map[string][]*storage.Session{}
	for _, s := range liveSessions {
		liveByHost[s.HostID] = append(liveByHost[s.HostID], s)
	}

	for _, host := range hosts {
		machine := topologyMachine{
			HostID:      host.ID,
			ProjectID:   host.ProjectID,
			Hostname:    host.Hostname,
			MachineID:   host.MachineID,
			OS:          host.OS,
			Fingerprint: host.Fingerprint,
			FirstSeenAt: host.FirstSeenAt.Format(time.RFC3339Nano),
			LastSeenAt:  host.LastSeenAt.Format(time.RFC3339Nano),
			Sessions:    []topologySession{},
		}
		// Cache the cheap-to-derive sysinfo fields off the host row.
		// Live CPU/mem land via /notify topology.machine_stats and
		// the frontend's reducer stamps them onto the snapshot in
		// place. We populate the slow-changing identity fields here
		// so the panel has something to render before the first
		// stats event lands.
		if host.PlatformVersion != "" || host.Platform != "" || host.KernelVersion != "" {
			machine.SysInfo = &topologySysInfo{
				KernelVersion:   host.KernelVersion,
				Platform:        host.Platform,
				PlatformVersion: host.PlatformVersion,
				MemTotalBytes:   host.MemTotalBytes,
			}
		}
		for _, s := range liveByHost[host.ID] {
			ts := topologySession{
				ID:          s.ID,
				User:        s.User,
				RemoteAddr:  s.RemoteAddr,
				Version:     s.Version,
				ConnectedAt: s.ConnectedAt.UTC().Format(time.RFC3339Nano),
				Active:      s.DisconnectedAt == nil,
			}
			if s.RemoteAddr != "" {
				info := ipinfo.Lookup(s.RemoteAddr)
				ts.RemoteInfo = &info
			}
			machine.Sessions = append(machine.Sessions, ts)
		}
		snap.Machines = append(snap.Machines, machine)
	}

	// 3. Live agent mesh nodes + reachability links. Only hosts that
	//    are currently in AgentLinkService get a node + edge — that's
	//    the thing the graph is uniquely positioned to surface vs. a
	//    flat table.
	if h.links != nil {
		for _, host := range hosts {
			if host.AgentID == "" {
				continue
			}
			if _, ok := h.links.Get(host.AgentID); !ok {
				continue
			}
			meshID := "agent:" + host.AgentID
			snap.MeshNodes = append(snap.MeshNodes, topologyMeshNodeRef{
				NodeID:    meshID,
				Kind:      "agent",
				HostID:    host.ID,
				ProjectID: pid,
			})
			snap.Links = append(snap.Links, topologyLink{
				A:        serverNodeID,
				B:        meshID,
				Up:       true,
				BytesIn:  0,
				BytesOut: 0,
				MsgsIn:   0,
				MsgsOut:  0,
			})
		}
	}

	c.JSON(http.StatusOK, snap)
}

// linkHistoryPoint is the JSON shape for a single row returned by
// GetLinkHistory. RttNs is only set when the sample recorded an RTT.
type linkHistoryPoint struct {
	At       time.Time `json:"at"`
	BytesIn  int64     `json:"bytes_in"`
	BytesOut int64     `json:"bytes_out"`
	MsgsIn   int64     `json:"msgs_in"`
	MsgsOut  int64     `json:"msgs_out"`
	RTTNs    *int64    `json:"rtt_ns,omitempty"`
}

// machineHistoryPoint is the JSON shape for a single row returned by
// GetMachineHistory.
type machineHistoryPoint struct {
	At         time.Time `json:"at"`
	CPUPercent *float64  `json:"cpu_percent,omitempty"`
	MemPercent *float64  `json:"mem_percent,omitempty"`
}

// GetLinkHistory handles
// GET /api/v1/projects/:pid/topology/links/:a/:b/stats?since=&until=&max=
//
// since / until are RFC3339 timestamps; when omitted the handler
// defaults to [now-1h, now]. max is an optional point cap — when set
// the repo thins the result to at most that many evenly-spaced
// samples.
func (h *TopologyHandler) GetLinkHistory(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusOK, gin.H{"points": []linkHistoryPoint{}})
		return
	}
	opts := parseHistoryOpts(c)
	rows, err := h.db.MeshStats().ListLinkHistory(
		c.Request.Context(),
		c.Param("pid"), c.Param("a"), c.Param("b"),
		opts,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list link history"})
		return
	}
	out := make([]linkHistoryPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, linkHistoryPoint{
			At: r.At, BytesIn: r.BytesIn, BytesOut: r.BytesOut,
			MsgsIn: r.MsgsIn, MsgsOut: r.MsgsOut, RTTNs: r.RTTNs,
		})
	}
	c.JSON(http.StatusOK, gin.H{"points": out})
}

// GetMachineHistory handles
// GET /api/v1/projects/:pid/topology/machines/:hid/stats?since=&until=&max=
func (h *TopologyHandler) GetMachineHistory(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusOK, gin.H{"points": []machineHistoryPoint{}})
		return
	}
	opts := parseHistoryOpts(c)
	rows, err := h.db.MeshStats().ListMachineHistory(
		c.Request.Context(), c.Param("hid"), opts,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list machine history"})
		return
	}
	out := make([]machineHistoryPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, machineHistoryPoint{
			At: r.At, CPUPercent: r.CPUPercent, MemPercent: r.MemPercent,
		})
	}
	c.JSON(http.StatusOK, gin.H{"points": out})
}

func parseHistoryOpts(c *gin.Context) storage.LinkHistoryOpts {
	now := time.Now().UTC()
	since := now.Add(-1 * time.Hour)
	until := now
	if v := c.Query("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = t
		}
	}
	if v := c.Query("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			until = t
		}
	}
	maxPoints := 0
	if v := c.Query("max"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPoints = n
		}
	}
	return storage.LinkHistoryOpts{Since: since, Until: until, MaxPoints: maxPoints}
}

// RegisterV1TopologyRoutes mounts the per-project topology routes.
// Viewer role is enough for every endpoint — all are read-only.
func RegisterV1TopologyRoutes(engine *gin.Engine, h *TopologyHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/topology")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleViewer))
	{
		grp.GET("", h.Get)
		grp.GET("/links/:a/:b/stats", h.GetLinkHistory)
		grp.GET("/machines/:hid/stats", h.GetMachineHistory)
	}
}
