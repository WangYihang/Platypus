package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

	// Security scan summary, populated only on the List endpoint
	// (single batched query) so HostCard renders the indicator
	// without an N+1 fan-out. Absent (nil pointer) = host has never
	// been scanned; UI distinguishes that from "scanned, all clean"
	// (counts present, all zero).
	SecuritySeverityCounts *storage.SeverityCounts `json:"security_severity_counts,omitempty"`
	SecurityScannedAtUnix  int64                   `json:"security_scanned_at_unix,omitempty"`
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
//
// One extra query merges per-host security-scan summaries so the
// fleet card grid can render the severity indicator without N+1
// requests. Hosts without a scan keep nil counts so the UI stays
// honest about "never scanned" vs "scanned and clean".
func (h *HostsHandler) List(c *gin.Context) {
	pid := c.Param("pid")
	hosts, err := h.db.Hosts().ListByProject(c.Request.Context(), pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list hosts"})
		return
	}
	summaries, err := h.db.SecurityScans().LatestSummariesForProject(c.Request.Context(), pid)
	if err != nil {
		// Don't fail the whole list response over the enrichment —
		// log and continue with empty summaries.
		log.L.Warn("hosts.list: scan summary fetch failed",
			"project_id", pid, "error", err.Error())
		summaries = nil
	}
	out := make([]hostResponse, 0, len(hosts))
	for _, host := range hosts {
		row := toHostResponse(host)
		if s, ok := summaries[host.ID]; ok {
			counts := s.Counts
			row.SecuritySeverityCounts = &counts
			row.SecurityScannedAtUnix = s.StartedAtUnix
		}
		out = append(out, row)
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

// --- Security scan ---

// scanResponse is the JSON shape returned by the per-host scan
// endpoints (POST + GET). Mirrors storage.SecurityScan + the
// findings list, with the proto-style snake_case fields the rest of
// the API uses.
type scanResponse struct {
	ID              string                 `json:"id"`
	HostID          string                 `json:"host_id"`
	ProjectID       string                 `json:"project_id"`
	StartedAtUnix   int64                  `json:"started_at_unix"`
	ElapsedMs       int64                  `json:"elapsed_ms"`
	Error           string                 `json:"error,omitempty"`
	SeverityCounts  storage.SeverityCounts `json:"severity_counts"`
	Findings        []findingResponse      `json:"findings"`
	// Checks travels as the raw JSON array stored on the row — the
	// shape (id/category/status/error/elapsed_ms/finding_count)
	// matches the agent's CheckResult proto and the UI consumes it
	// directly. We use json.RawMessage so the server doesn't pay
	// the unmarshal/remarshal cost on a payload it doesn't touch.
	ChecksRaw json.RawMessage `json:"checks"`
}

type scanSummaryResponse struct {
	ID             string                 `json:"id"`
	StartedAtUnix  int64                  `json:"started_at_unix"`
	ElapsedMs      int64                  `json:"elapsed_ms"`
	Error          string                 `json:"error,omitempty"`
	SeverityCounts storage.SeverityCounts `json:"severity_counts"`
}

type findingResponse struct {
	ID            string   `json:"id"`            // storage row id
	FindingID     string   `json:"finding_id"`    // stable id like "ssh.permitrootlogin"
	CheckID       string   `json:"check_id"`
	Category      string   `json:"category"`
	Severity      string   `json:"severity"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Evidence      string   `json:"evidence"`
	Remediation   string   `json:"remediation"`
	References    []string `json:"references,omitempty"`
	HostID        string   `json:"host_id,omitempty"`
	ScannedAtUnix int64    `json:"scanned_at_unix,omitempty"`
}

func toFindingResponse(f *storage.SecurityFinding, includeContext bool) findingResponse {
	out := findingResponse{
		ID:          f.ID,
		FindingID:   f.FindingID,
		CheckID:     f.CheckID,
		Category:    f.Category,
		Severity:    f.Severity,
		Title:       f.Title,
		Description: f.Description,
		Evidence:    f.Evidence,
		Remediation: f.Remediation,
	}
	if f.ReferencesJSON != "" && f.ReferencesJSON != "null" {
		var refs []string
		if err := json.Unmarshal([]byte(f.ReferencesJSON), &refs); err == nil {
			out.References = refs
		}
	}
	if includeContext {
		out.HostID = f.HostID
		out.ScannedAtUnix = f.ScannedAtUnix
	}
	return out
}

func toScanResponse(scan *storage.SecurityScan, findings []*storage.SecurityFinding) scanResponse {
	checks := json.RawMessage(scan.ChecksJSON)
	if len(checks) == 0 {
		checks = json.RawMessage("[]")
	}
	out := scanResponse{
		ID:             scan.ID,
		HostID:         scan.HostID,
		ProjectID:      scan.ProjectID,
		StartedAtUnix:  scan.StartedAtUnix,
		ElapsedMs:      scan.ElapsedMs,
		Error:          scan.Error,
		SeverityCounts: scan.SeverityCounts,
		Findings:       make([]findingResponse, 0, len(findings)),
		ChecksRaw:      checks,
	}
	for _, f := range findings {
		out.Findings = append(out.Findings, toFindingResponse(f, false))
	}
	return out
}

// rescanRequest is the optional JSON body for POST .../security-scan.
// Empty body is fine — the agent runs every registered checker.
type rescanRequest struct {
	CheckIDs          []string `json:"check_ids,omitempty"`
	Categories        []string `json:"categories,omitempty"`
	PerCheckTimeoutMs uint32   `json:"per_check_timeout_ms,omitempty"`
}

// RescanHost handles POST /projects/:pid/hosts/:hid/security-scan.
// Triggers a fresh SecurityScan RPC against the connected agent,
// persists the result, and returns it.
//
// Operator role required: this both calls into the agent (which can
// be expensive — file walks, /proc reads) and writes a DB row that
// shows up on the audit-adjacent hosts list. Viewer is too low.
func (h *HostsHandler) RescanHost(c *gin.Context) {
	start := time.Now()
	pid := c.Param("pid")
	hid := c.Param("hid")
	log.L.Info("http_security_scan_enter", "project_id", pid, "host_id", hid)

	var findingCount int
	var persisted bool
	defer func() {
		log.L.Info("http_security_scan_exit",
			"project_id", pid, "host_id", hid,
			"status", c.Writer.Status(),
			"finding_count", findingCount,
			"persisted", persisted,
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
	}()

	if h.svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "live security scan not configured"})
		return
	}

	host, err := h.db.Hosts().GetByID(c.Request.Context(), hid)
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

	var body rescanRequest
	_ = c.ShouldBindJSON(&body)

	resp, err := core.CallAgentRPC(c.Request.Context(), h.svc, host.AgentID, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_SecurityScan{SecurityScan: &v2pb.SecurityScanRequest{
			CheckIds:          body.CheckIDs,
			Categories:        body.Categories,
			PerCheckTimeoutMs: body.PerCheckTimeoutMs,
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
	scanProto := resp.GetSecurityScan()
	if scanProto == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "agent returned empty scan response"})
		return
	}

	scan, findings := buildStorageRows(pid, host.ID, scanProto)
	findingCount = len(findings)
	if err := h.db.SecurityScans().Save(c.Request.Context(), scan, findings); err != nil {
		log.L.Warn("http_security_scan: persist failed",
			"host_id", host.ID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist scan"})
		return
	}
	persisted = true

	c.JSON(http.StatusOK, toScanResponse(scan, findings))
}

// GetSecurityScan handles GET /projects/:pid/hosts/:hid/security-scan
// and returns the latest persisted scan + findings, or 404 when the
// host has never been scanned. Read-only, viewer role.
//
// Optional ?scan_id=... selects a specific historical scan (used by
// the per-host History dropdown).
func (h *HostsHandler) GetSecurityScan(c *gin.Context) {
	pid := c.Param("pid")
	hid := c.Param("hid")
	host, err := h.db.Hosts().GetByID(c.Request.Context(), hid)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && host.ProjectID != pid) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup host"})
		return
	}

	var scan *storage.SecurityScan
	var findings []*storage.SecurityFinding
	if scanID := c.Query("scan_id"); scanID != "" {
		scan, findings, err = h.db.SecurityScans().GetScan(c.Request.Context(), scanID)
		// Defensive: a scan id leaked from a different host or
		// project must not surface here. ErrNotFound from the wrong
		// project is more honest than 200 with a stranger's data.
		if err == nil && (scan.HostID != host.ID || scan.ProjectID != pid) {
			err = storage.ErrNotFound
		}
	} else {
		scan, findings, err = h.db.SecurityScans().LatestForHost(c.Request.Context(), host.ID)
	}
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host has never been scanned"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load scan"})
		return
	}
	c.JSON(http.StatusOK, toScanResponse(scan, findings))
}

// ListSecurityScans handles GET /projects/:pid/hosts/:hid/security-scans
// and returns lightweight summary rows (no findings) newest-first.
// Optional ?limit=N (default 10, max 50).
func (h *HostsHandler) ListSecurityScans(c *gin.Context) {
	pid := c.Param("pid")
	hid := c.Param("hid")
	host, err := h.db.Hosts().GetByID(c.Request.Context(), hid)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && host.ProjectID != pid) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup host"})
		return
	}
	limit := 10
	if s := c.Query("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			limit = n
		}
	}
	scans, err := h.db.SecurityScans().ListScansForHost(c.Request.Context(), host.ID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list scans"})
		return
	}
	out := make([]scanSummaryResponse, 0, len(scans))
	for _, s := range scans {
		out = append(out, scanSummaryResponse{
			ID:             s.ID,
			StartedAtUnix:  s.StartedAtUnix,
			ElapsedMs:      s.ElapsedMs,
			Error:          s.Error,
			SeverityCounts: s.SeverityCounts,
		})
	}
	c.JSON(http.StatusOK, gin.H{"scans": out})
}

// buildStorageRows turns an agent SecurityScanResponse into the
// storage rows we'll persist. Generates fresh row UUIDs so two
// concurrent scans on the same host can't collide. Severity counts
// on the scan row are recomputed by storage.Save from the findings,
// so we leave those zero here.
func buildStorageRows(projectID, hostID string, src *v2pb.SecurityScanResponse) (*storage.SecurityScan, []*storage.SecurityFinding) {
	scanID := uuid.NewString()
	checksJSON := []byte("[]")
	if checks := src.GetChecks(); len(checks) > 0 {
		// Convert each proto CheckResult to a small JSON-friendly
		// struct. The shape mirrors the proto; defining it inline
		// keeps the wire shape close to the storage shape.
		type checkRow struct {
			ID           string `json:"id"`
			Category     string `json:"category"`
			Status       string `json:"status"`
			Error        string `json:"error,omitempty"`
			ElapsedMs    uint64 `json:"elapsed_ms"`
			FindingCount uint32 `json:"finding_count"`
		}
		rows := make([]checkRow, 0, len(checks))
		for _, c := range checks {
			rows = append(rows, checkRow{
				ID:           c.GetId(),
				Category:     c.GetCategory(),
				Status:       c.GetStatus(),
				Error:        c.GetError(),
				ElapsedMs:    c.GetElapsedMs(),
				FindingCount: c.GetFindingCount(),
			})
		}
		if b, err := json.Marshal(rows); err == nil {
			checksJSON = b
		}
	}
	scan := &storage.SecurityScan{
		ID:            scanID,
		ProjectID:     projectID,
		HostID:        hostID,
		StartedAtUnix: src.GetStartedAtUnix(),
		ElapsedMs:     int64(src.GetElapsedMs()),
		Error:         src.GetError(),
		ChecksJSON:    string(checksJSON),
	}
	findings := make([]*storage.SecurityFinding, 0, len(src.GetFindings()))
	for _, f := range src.GetFindings() {
		refsJSON := "[]"
		if refs := f.GetReferences(); len(refs) > 0 {
			if b, err := json.Marshal(refs); err == nil {
				refsJSON = string(b)
			}
		}
		findings = append(findings, &storage.SecurityFinding{
			ID:             uuid.NewString(),
			ScanID:         scanID,
			HostID:         hostID,
			ProjectID:      projectID,
			FindingID:      f.GetId(),
			CheckID:        f.GetCheckId(),
			Category:       f.GetCategory(),
			Severity:       f.GetSeverity(),
			Title:          f.GetTitle(),
			Description:    f.GetDescription(),
			Evidence:       f.GetEvidence(),
			Remediation:    f.GetRemediation(),
			ReferencesJSON: refsJSON,
		})
	}
	return scan, findings
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
		viewer.GET("/:hid/security-scan", h.GetSecurityScan)
		viewer.GET("/:hid/security-scans", h.ListSecurityScans)
	}

	// Re-scan triggers an agent RPC and writes a new DB row, so it
	// sits one tier above Get/List in the role gate.
	operator := engine.Group("/api/v1/projects/:pid/hosts")
	operator.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleOperator))
	{
		operator.POST("/:hid/security-scan", h.RescanHost)
	}

	admin := engine.Group("/api/v1/projects/:pid/hosts")
	admin.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		admin.POST("/:hid/approve", h.Approve)
		admin.POST("/:hid/reject", h.Reject)
	}
}
