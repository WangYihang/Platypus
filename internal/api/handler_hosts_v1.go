package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/ipinfo"
	"github.com/WangYihang/Platypus/internal/log"
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

	ApprovalStatus    string     `json:"approval_status"`
	ApprovalDecidedAt *time.Time `json:"approval_decided_at,omitempty"`
	ApprovalDecidedBy string     `json:"approval_decided_by,omitempty"`
	ApprovalReason    string     `json:"approval_reason,omitempty"`

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
	PrimaryIP       string       `json:"primary_ip,omitempty"`
	PrimaryIPInfo   *ipinfo.Info `json:"primary_ip_info,omitempty"`
	PrimaryMAC      string       `json:"primary_mac,omitempty"`
	BootTimeUnix    int64        `json:"boot_time_unix,omitempty"`

	EgressIP     string       `json:"egress_ip,omitempty"`
	EgressIPInfo *ipinfo.Info `json:"egress_ip_info,omitempty"`
	PublicIP     string       `json:"public_ip,omitempty"`
	PublicIPInfo *ipinfo.Info `json:"public_ip_info,omitempty"`

	BuildVersion    string `json:"build_version,omitempty"`
	BuildCommit     string `json:"build_commit,omitempty"`
	BuildDate       string `json:"build_date,omitempty"`
	ProtocolVersion uint32 `json:"protocol_version,omitempty"`

	MachineType   string `json:"machine_type,omitempty"`
	ChassisType   string `json:"chassis_type,omitempty"`
	ProductVendor string `json:"product_vendor,omitempty"`
	ProductName   string `json:"product_name,omitempty"`
	BIOSVendor    string `json:"bios_vendor,omitempty"`
	BIOSVersion   string `json:"bios_version,omitempty"`
	GPUSummary    string `json:"gpu_summary,omitempty"`
}

func toHostResponse(h *storage.Host) hostResponse {
	var primaryInfo *ipinfo.Info
	if h.PrimaryIP != "" {
		info := ipinfo.Lookup(h.PrimaryIP)
		primaryInfo = &info
	}
	var egressInfo *ipinfo.Info
	if h.EgressIP != "" {
		info := ipinfo.Lookup(h.EgressIP)
		egressInfo = &info
	}
	var publicInfo *ipinfo.Info
	if h.PublicIP != "" {
		info := ipinfo.Lookup(h.PublicIP)
		publicInfo = &info
	}
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
		PrimaryIPInfo:       primaryInfo,
		PrimaryMAC:          h.PrimaryMAC,
		BootTimeUnix:        h.BootTimeUnix,
		EgressIP:            h.EgressIP,
		EgressIPInfo:        egressInfo,
		PublicIP:            h.PublicIP,
		PublicIPInfo:        publicInfo,
		BuildVersion:        h.BuildVersion,
		BuildCommit:         h.BuildCommit,
		BuildDate:           h.BuildDate,
		ProtocolVersion:     h.ProtocolVersion,
		MachineType:         h.MachineType,
		ChassisType:         h.ChassisType,
		ProductVendor:       h.ProductVendor,
		ProductName:         h.ProductName,
		BIOSVendor:          h.BIOSVendor,
		BIOSVersion:         h.BIOSVersion,
		GPUSummary:          h.GPUSummary,
		ApprovalStatus:      string(h.ApprovalStatus),
		ApprovalDecidedAt:   h.ApprovalDecidedAt,
		ApprovalDecidedBy:   h.ApprovalDecidedBy,
		ApprovalReason:      h.ApprovalReason,
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
	start := time.Now()
	log.L.Info("http_sysinfo_enter",
		"project_id", c.Param("pid"),
		"host_id", c.Param("hid"),
	)
	defer func() {
		log.L.Info("http_sysinfo_exit",
			"project_id", c.Param("pid"),
			"host_id", c.Param("hid"),
			"status", c.Writer.Status(),
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
	}()
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

// GetProcesses handles GET /projects/:pid/hosts/:hid/processes and
// proxies a live ProcessList RPC through to the connected agent. The
// response body is the raw v2pb.ProcessListResponse so the Web UI
// sees exactly the shape the agent produced — see GetSysInfo for the
// same design rationale. Query parameters:
//
//	top   — 0 or absent = as many as the 500-row cap allows
//	sort  — "cpu" (default) | "mem" | "rss" | "pid"
//
// Viewer role is sufficient: process enumeration is read-only and
// already lets operators see what's running via the Terminal tab.
func (h *HostsHandler) GetProcesses(c *gin.Context) {
	start := time.Now()
	var topN uint32
	if s := c.Query("top"); s != "" {
		if n, err := strconv.ParseUint(s, 10, 32); err == nil {
			topN = uint32(n)
		}
	}
	sortBy := c.DefaultQuery("sort", "cpu")
	log.L.Info("http_processes_enter",
		"project_id", c.Param("pid"),
		"host_id", c.Param("hid"),
		"top", topN,
		"sort", sortBy,
	)
	var returned int
	var totalCount uint32
	defer func() {
		log.L.Info("http_processes_exit",
			"project_id", c.Param("pid"),
			"host_id", c.Param("hid"),
			"status", c.Writer.Status(),
			"returned", returned,
			"total_count", totalCount,
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
	}()

	if h.svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "live process list not configured"})
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
		Payload: &v2pb.RpcRequest_ProcessList{ProcessList: &v2pb.ProcessListRequest{
			TopN:   topN,
			SortBy: sortBy,
		}},
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
	pl := resp.GetProcessList()
	if pl != nil {
		returned = len(pl.Processes)
		totalCount = pl.TotalCount
	}
	c.JSON(http.StatusOK, pl)
}

// approvalDecisionRequest is the JSON body for Approve / Reject. The
// reason field is free-form and lands in the activity meta + the
// hosts.approval_reason column for audit. Empty is fine.
type approvalDecisionRequest struct {
	Reason string `json:"reason"`
}

// ListPendingApprovals handles GET /projects/:pid/hosts/pending. Returns
// hosts in the project still awaiting admin approval, oldest first so
// the operator works through the queue in arrival order. Surfaces
// fingerprint + reported OS / IP so an admin can sanity-check "is
// this expected?" before clicking Approve.
func (h *HostsHandler) ListPendingApprovals(c *gin.Context) {
	hosts, err := h.db.Hosts().ListPendingByProject(c.Request.Context(), c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list pending approvals"})
		return
	}
	out := make([]hostResponse, 0, len(hosts))
	for _, host := range hosts {
		out = append(out, toHostResponse(host))
	}
	c.JSON(http.StatusOK, gin.H{"hosts": out})
}

// PendingApprovalCount handles GET /projects/:pid/hosts/pending/count.
// Cheap COUNT(*) for the top-bar badge — separate endpoint so the
// frontend doesn't pay the full row scan + JSON marshal cost on every
// poll.
func (h *HostsHandler) PendingApprovalCount(c *gin.Context) {
	n, err := h.db.Hosts().CountPendingByProject(c.Request.Context(), c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count pending approvals"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"pending": n})
}

// Approve handles POST /projects/:pid/hosts/:hid/approve. Flips the
// host from `pending` (or `rejected` — admins can change their mind)
// to `approved` and stamps the decision metadata. The next agent link
// attempt will succeed.
func (h *HostsHandler) Approve(c *gin.Context) {
	projectID := c.Param("pid")
	hostID := c.Param("hid")
	claims, _ := ClaimsFromContext(c)

	var body approvalDecisionRequest
	// Body is optional — admins clicking the button get an empty
	// payload. ShouldBindJSON treats EOF as ok when the struct has
	// no required fields.
	_ = c.ShouldBindJSON(&body)

	now := time.Now().UTC()
	err := h.db.Hosts().Approve(c.Request.Context(), hostID, claims.UserID, body.Reason, now)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		log.Warn("hosts: approve %s: %v", hostID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "approve host"})
		return
	}

	pid := projectID
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryAdmin,
		Action:      "host.approve",
		TargetType:  "host",
		TargetID:    hostID,
		TargetLabel: hostID,
		Outcome:     storage.OutcomeSuccess,
		Meta:        map[string]string{"reason": body.Reason},
		At:          now,
	})
	c.JSON(http.StatusOK, gin.H{"status": "approved"})
}

// Reject handles POST /projects/:pid/hosts/:hid/reject. Flips the host
// to `rejected`. The agent's open WS link (if any) stays up — operators
// who want a hard kick should also revoke the cert via the existing
// /pat-tokens admin surface; the link gate will refuse the next
// reconnect.
func (h *HostsHandler) Reject(c *gin.Context) {
	projectID := c.Param("pid")
	hostID := c.Param("hid")
	claims, _ := ClaimsFromContext(c)

	var body approvalDecisionRequest
	_ = c.ShouldBindJSON(&body)

	now := time.Now().UTC()
	err := h.db.Hosts().Reject(c.Request.Context(), hostID, claims.UserID, body.Reason, now)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		log.Warn("hosts: reject %s: %v", hostID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reject host"})
		return
	}

	pid := projectID
	RecordActivity(c, ActivityInput{
		ProjectID:   &pid,
		Category:    storage.CategoryAdmin,
		Action:      "host.reject",
		TargetType:  "host",
		TargetID:    hostID,
		TargetLabel: hostID,
		Outcome:     storage.OutcomeSuccess,
		Meta:        map[string]string{"reason": body.Reason},
		At:          now,
	})
	c.JSON(http.StatusOK, gin.H{"status": "rejected"})
}

// RegisterV1HostsRoutes mounts the per-project host routes under
// /api/v1/projects/:pid/hosts. Every route is RequireAuth +
// RequireProjectRole(viewer); approve/reject bump up to admin.
func RegisterV1HostsRoutes(engine *gin.Engine, h *HostsHandler, rbac *RBAC) {
	viewer := engine.Group("/api/v1/projects/:pid/hosts")
	viewer.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleViewer))
	{
		viewer.GET("", h.List)
		viewer.GET("/pending", h.ListPendingApprovals)
		viewer.GET("/pending/count", h.PendingApprovalCount)
		viewer.GET("/:hid", h.Get)
		viewer.GET("/:hid/sysinfo", h.GetSysInfo)
		viewer.GET("/:hid/processes", h.GetProcesses)
	}

	admin := engine.Group("/api/v1/projects/:pid/hosts")
	admin.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		admin.POST("/:hid/approve", h.Approve)
		admin.POST("/:hid/reject", h.Reject)
	}
}
