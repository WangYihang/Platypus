package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HostsHandler serves per-project host aggregation routes. Host writes
// happen out-of-band (agent handshake) — this handler is read-only.
type HostsHandler struct {
	db  *storage.DB
	svc *core.AgentLinkService
}

func NewHostsHandler(db *storage.DB) *HostsHandler {
	return &HostsHandler{db: db}
}

// WithAgentLinks attaches the live-link registry so GetSysInfo can
// reach the agent behind a host_id. Optional; routes that don't need
// live lookup keep working without it.
func (h *HostsHandler) WithAgentLinks(svc *core.AgentLinkService) *HostsHandler {
	h.svc = svc
	return h
}

// hostResponse is the JSON shape of a Host on the wire. Keeps the internal
// HostIdentity fields out of the response and stamps booleans in the
// underscore_case expected by the frontend. Extended fields (arch,
// kernel, CPU, memory, etc.) come from the last SysInfo snapshot the
// agent shared with the server — they're populated at enrollment and
// refreshed on every agent reconnect.
type hostResponse struct {
	ID                  string    `json:"id"`
	ProjectID           string    `json:"project_id"`
	MachineID           string    `json:"machine_id,omitempty"`
	Fingerprint         string    `json:"fingerprint"`
	FingerprintFallback bool      `json:"fingerprint_fallback"`
	Hostname            string    `json:"hostname,omitempty"`
	PrimaryAlias        string    `json:"primary_alias,omitempty"`
	OS                  string    `json:"os,omitempty"`
	FirstSeenAt         time.Time `json:"first_seen_at"`
	LastSeenAt          time.Time `json:"last_seen_at"`

	AgentID         string `json:"agent_id,omitempty"`
	Arch            string `json:"arch,omitempty"`
	Platform        string `json:"platform,omitempty"`
	PlatformFamily  string `json:"platform_family,omitempty"`
	PlatformVersion string `json:"platform_version,omitempty"`
	KernelVersion   string `json:"kernel_version,omitempty"`
	CPUModel        string `json:"cpu_model,omitempty"`
	NumCPU          int    `json:"num_cpu,omitempty"`
	MemTotalBytes   int64  `json:"mem_total_bytes,omitempty"`
	CurrentUser     string `json:"current_user,omitempty"`
	Timezone        string `json:"timezone,omitempty"`
	PrimaryIP       string `json:"primary_ip,omitempty"`
	PrimaryMAC      string `json:"primary_mac,omitempty"`
	BootTimeUnix    int64  `json:"boot_time_unix,omitempty"`
	AgentVersion    string `json:"agent_version,omitempty"`
}

func toHostResponse(h *storage.Host) hostResponse {
	return hostResponse{
		ID:                  h.ID,
		ProjectID:           h.ProjectID,
		MachineID:           h.MachineID,
		Fingerprint:         h.Fingerprint,
		FingerprintFallback: h.FingerprintFallback,
		Hostname:            h.Hostname,
		PrimaryAlias:        h.PrimaryAlias,
		OS:                  h.OS,
		FirstSeenAt:         h.FirstSeenAt,
		LastSeenAt:          h.LastSeenAt,
		AgentID:             h.AgentID,
		Arch:                h.Arch,
		Platform:            h.Platform,
		PlatformFamily:      h.PlatformFamily,
		PlatformVersion:     h.PlatformVersion,
		KernelVersion:       h.KernelVersion,
		CPUModel:            h.CPUModel,
		NumCPU:              h.NumCPU,
		MemTotalBytes:       h.MemTotalBytes,
		CurrentUser:         h.CurrentUser,
		Timezone:            h.Timezone,
		PrimaryIP:           h.PrimaryIP,
		PrimaryMAC:          h.PrimaryMAC,
		BootTimeUnix:        h.BootTimeUnix,
		AgentVersion:        h.AgentVersion,
	}
}

// List handles GET /projects/:pid/hosts. Gated by RequireProjectRole(viewer),
// so non-members already got a 403 before we got here.
func (h *HostsHandler) List(c *gin.Context) {
	hosts, err := h.db.Hosts().ListByProject(c.Request.Context(), c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list hosts"})
		return
	}
	out := make([]hostResponse, 0, len(hosts))
	for _, host := range hosts {
		out = append(out, toHostResponse(host))
	}
	c.JSON(http.StatusOK, gin.H{"hosts": out})
}

// Get handles GET /projects/:pid/hosts/:hid. A host in project A must not
// be reachable via project B's URL — enforced by checking host.ProjectID
// against :pid and returning 404 on mismatch.
func (h *HostsHandler) Get(c *gin.Context) {
	pid := c.Param("pid")
	host, err := h.db.Hosts().GetByID(c.Request.Context(), c.Param("hid"))
	if errors.Is(err, storage.ErrNotFound) || (err == nil && host.ProjectID != pid) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup host"})
		return
	}
	c.JSON(http.StatusOK, toHostResponse(host))
}

// GetSysInfo handles GET /projects/:pid/hosts/:hid/sysinfo and
// proxies a live SysInfo RPC through to the agent currently holding
// this host's agent_id. Responses are the raw v2pb.SysInfoResponse
// JSON so the Web UI gets exactly the shape the agent produced —
// no server-side flattening. When the agent is offline the caller
// gets 404 with "agent not connected" so the UI can fall back to
// the cached hostResponse fields. Viewer role is sufficient: this
// is read-only instrumentation, same tier as the host row itself.
func (h *HostsHandler) GetSysInfo(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "live sys info not configured"})
		return
	}
	pid := c.Param("pid")
	host, err := h.db.Hosts().GetByID(c.Request.Context(), c.Param("hid"))
	if errors.Is(err, storage.ErrNotFound) || (err == nil && host.ProjectID != pid) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup host"})
		return
	}
	if host.AgentID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "host has no registered agent"})
		return
	}
	resp, err := core.CallAgentRPC(c.Request.Context(), h.svc, host.AgentID, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_SysInfo{SysInfo: &v2pb.SysInfoRequest{}},
	})
	if err != nil {
		var notConnected *core.ErrAgentNotConnected
		if errors.As(err, &notConnected) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not connected"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if resp.Error != "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": resp.Error})
		return
	}
	c.JSON(http.StatusOK, resp.GetSysInfo())
}

// RegisterV1HostsRoutes mounts the per-project host routes under
// /api/v1/projects/:pid/hosts. Every route is RequireAuth +
// RequireProjectRole(viewer).
func RegisterV1HostsRoutes(engine *gin.Engine, h *HostsHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/hosts")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleViewer))
	{
		grp.GET("", h.List)
		grp.GET("/:hid", h.Get)
		grp.GET("/:hid/sysinfo", h.GetSysInfo)
	}
}
