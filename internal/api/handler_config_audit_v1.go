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
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// --- Config audit (sensitive-info leak detection) ---
//
// The handlers here are the sensitive-info-leak sibling of the
// security scan endpoints in handler_hosts_v1.go. They share the
// HostsHandler struct (so they get the same DB + AgentLinkService)
// but live in their own file because the request/response shapes,
// the storage tables, and the RPC payloads are independent.
//
// The wire types deliberately mirror the security ones (same field
// naming style, same status taxonomy) so the frontend can reuse most
// of its rendering, but the keys differ (`risk` vs `severity`,
// `leaks` vs `findings`, `auditors` vs `checks`) so a typo doesn't
// silently merge the two domains in either direction.

type auditResponse struct {
	ID            string             `json:"id"`
	HostID        string             `json:"host_id"`
	ProjectID     string             `json:"project_id"`
	StartedAtUnix int64              `json:"started_at_unix"`
	ElapsedMs     int64              `json:"elapsed_ms"`
	Error         string             `json:"error,omitempty"`
	RiskCounts    storage.RiskCounts `json:"risk_counts"`
	Leaks         []leakResponse     `json:"leaks"`
	// Auditors travels as the raw JSON the storage layer keeps; the
	// shape (id/category/status/error/elapsed_ms/leak_count) matches
	// the agent's AuditorResult proto.
	AuditorsRaw json.RawMessage `json:"auditors"`
}

type auditSummaryResponse struct {
	ID            string             `json:"id"`
	StartedAtUnix int64              `json:"started_at_unix"`
	ElapsedMs     int64              `json:"elapsed_ms"`
	Error         string             `json:"error,omitempty"`
	RiskCounts    storage.RiskCounts `json:"risk_counts"`
}

type leakResponse struct {
	ID            string   `json:"id"`
	LeakID        string   `json:"leak_id"`
	AuditorID     string   `json:"auditor_id"`
	Category      string   `json:"category"`
	Risk          string   `json:"risk"`
	Title         string   `json:"title"`
	Location      string   `json:"location"`
	Match         string   `json:"match"` // already redacted on the agent
	Pattern       string   `json:"pattern"`
	Description   string   `json:"description"`
	Remediation   string   `json:"remediation"`
	References    []string `json:"references,omitempty"`
	HostID        string   `json:"host_id,omitempty"`
	ScannedAtUnix int64    `json:"scanned_at_unix,omitempty"`
}

type availableAuditorResponse struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Applicable  bool     `json:"applicable"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	References  []string `json:"references,omitempty"`
}

func toLeakResponse(l *storage.ConfigLeak, includeContext bool) leakResponse {
	out := leakResponse{
		ID:          l.ID,
		LeakID:      l.LeakID,
		AuditorID:   l.AuditorID,
		Category:    l.Category,
		Risk:        l.Risk,
		Title:       l.Title,
		Location:    l.Location,
		Match:       l.MatchRedacted,
		Pattern:     l.Pattern,
		Description: l.Description,
		Remediation: l.Remediation,
	}
	if l.ReferencesJSON != "" && l.ReferencesJSON != "null" {
		var refs []string
		if err := json.Unmarshal([]byte(l.ReferencesJSON), &refs); err == nil {
			out.References = refs
		}
	}
	if includeContext {
		out.HostID = l.HostID
		out.ScannedAtUnix = l.ScannedAtUnix
	}
	return out
}

func toAuditResponse(audit *storage.ConfigAudit, leaks []*storage.ConfigLeak) auditResponse {
	auditors := json.RawMessage(audit.AuditorsJSON)
	if len(auditors) == 0 {
		auditors = json.RawMessage("[]")
	}
	out := auditResponse{
		ID:            audit.ID,
		HostID:        audit.HostID,
		ProjectID:     audit.ProjectID,
		StartedAtUnix: audit.StartedAtUnix,
		ElapsedMs:     audit.ElapsedMs,
		Error:         audit.Error,
		RiskCounts:    audit.RiskCounts,
		Leaks:         make([]leakResponse, 0, len(leaks)),
		AuditorsRaw:   auditors,
	}
	for _, l := range leaks {
		out.Leaks = append(out.Leaks, toLeakResponse(l, false))
	}
	return out
}

// reauditRequest is the optional JSON body for POST .../config-audit.
// Empty body = run every registered auditor.
type reauditRequest struct {
	AuditorIDs          []string `json:"auditor_ids,omitempty"`
	Categories          []string `json:"categories,omitempty"`
	PerAuditorTimeoutMs uint32   `json:"per_auditor_timeout_ms,omitempty"`
}

// ReauditHost handles POST /projects/:pid/hosts/:hid/config-audit.
// Triggers a fresh ConfigAudit RPC against the connected agent,
// merges with any prior audit (so a partial re-run keeps the other
// auditors' leaks), persists the result, and returns it.
//
// Operator role required: same reasoning as RescanHost — the call
// drives non-trivial agent work (filesystem walks, /proc reads) and
// writes a row that shows up on aggregation pages.
func (h *HostsHandler) ReauditHost(c *gin.Context) {
	start := time.Now()
	pid := c.Param("pid")
	hid := c.Param("hid")
	log.L.Info("http_config_audit_enter", "project_id", pid, "host_id", hid)

	var leakCount int
	var persisted bool
	defer func() {
		log.L.Info("http_config_audit_exit",
			"project_id", pid, "host_id", hid,
			"status", c.Writer.Status(),
			"leak_count", leakCount,
			"persisted", persisted,
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
	}()

	if h.svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "live config audit not configured"})
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

	var body reauditRequest
	_ = c.ShouldBindJSON(&body)

	resp, err := core.CallAgentRPC(c.Request.Context(), h.svc, host.AgentID, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_ConfigAudit{ConfigAudit: &v2pb.ConfigAuditRequest{
			AuditorIds:          body.AuditorIDs,
			Categories:          body.Categories,
			PerAuditorTimeoutMs: body.PerAuditorTimeoutMs,
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
	auditProto := resp.GetConfigAudit()
	if auditProto == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "agent returned empty audit response"})
		return
	}

	audit, leaks := buildConfigAuditRows(pid, host.ID, auditProto)

	partial := len(body.AuditorIDs) > 0 || len(body.Categories) > 0
	if partial {
		if prev, prevLeaks, prevErr := h.db.ConfigAudits().LatestForHost(c.Request.Context(), host.ID); prevErr == nil {
			audit, leaks = mergePartialAudit(audit, leaks, prev, prevLeaks, auditProto)
		} else if !errors.Is(prevErr, storage.ErrNotFound) {
			log.L.Warn("http_config_audit: load prior audit for merge failed",
				"host_id", host.ID, "error", prevErr.Error())
		}
	}

	leakCount = len(leaks)
	if err := h.db.ConfigAudits().Save(c.Request.Context(), audit, leaks); err != nil {
		log.L.Warn("http_config_audit: persist failed",
			"host_id", host.ID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist audit"})
		return
	}
	persisted = true

	c.JSON(http.StatusOK, toAuditResponse(audit, leaks))
}

// GetConfigAudit handles GET /projects/:pid/hosts/:hid/config-audit
// and returns the latest persisted audit, or 404 when the host has
// never been audited. Optional ?audit_id=... selects a historical
// audit (used by the per-host History dropdown).
func (h *HostsHandler) GetConfigAudit(c *gin.Context) {
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

	var audit *storage.ConfigAudit
	var leaks []*storage.ConfigLeak
	if auditID := c.Query("audit_id"); auditID != "" {
		audit, leaks, err = h.db.ConfigAudits().GetAudit(c.Request.Context(), auditID)
		if err == nil && (audit.HostID != host.ID || audit.ProjectID != pid) {
			err = storage.ErrNotFound
		}
	} else {
		audit, leaks, err = h.db.ConfigAudits().LatestForHost(c.Request.Context(), host.ID)
	}
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "host has never been audited"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load audit"})
		return
	}
	c.JSON(http.StatusOK, toAuditResponse(audit, leaks))
}

// ListConfigAudits handles GET /projects/:pid/hosts/:hid/config-audits
// and returns lightweight audit summaries newest-first.
func (h *HostsHandler) ListConfigAudits(c *gin.Context) {
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
	audits, err := h.db.ConfigAudits().ListAuditsForHost(c.Request.Context(), host.ID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list audits"})
		return
	}
	out := make([]auditSummaryResponse, 0, len(audits))
	for _, a := range audits {
		out = append(out, auditSummaryResponse{
			ID:            a.ID,
			StartedAtUnix: a.StartedAtUnix,
			ElapsedMs:     a.ElapsedMs,
			Error:         a.Error,
			RiskCounts:    a.RiskCounts,
		})
	}
	c.JSON(http.StatusOK, gin.H{"audits": out})
}

// ListAvailableAuditors handles GET /projects/:pid/hosts/:hid/config-auditors.
// Proxies a live ListConfigAuditors RPC so the UI gets the full set
// of registered auditors regardless of whether any audit has run.
func (h *HostsHandler) ListAvailableAuditors(c *gin.Context) {
	pid := c.Param("pid")
	hid := c.Param("hid")
	if h.svc == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "live config audit not configured"})
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
	resp, err := core.CallAgentRPC(c.Request.Context(), h.svc, host.AgentID, &v2pb.RpcRequest{
		Payload: &v2pb.RpcRequest_ListConfigAuditors{ListConfigAuditors: &v2pb.ListConfigAuditorsRequest{}},
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
	listProto := resp.GetListConfigAuditors()
	if listProto == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "agent returned empty auditors response"})
		return
	}
	out := make([]availableAuditorResponse, 0, len(listProto.GetAuditors()))
	for _, a := range listProto.GetAuditors() {
		out = append(out, availableAuditorResponse{
			ID:          a.GetId(),
			Category:    a.GetCategory(),
			Applicable:  a.GetApplicable(),
			Title:       a.GetTitle(),
			Description: a.GetDescription(),
			References:  append([]string(nil), a.GetReferences()...),
		})
	}
	c.JSON(http.StatusOK, gin.H{"auditors": out})
}

// buildConfigAuditRows turns a ConfigAuditResponse into storage rows.
// Mirrors buildStorageRows in handler_hosts_v1.go but for the audit
// schema. Generates fresh row UUIDs so concurrent re-audits on the
// same host can't collide.
func buildConfigAuditRows(projectID, hostID string, src *v2pb.ConfigAuditResponse) (*storage.ConfigAudit, []*storage.ConfigLeak) {
	auditID := uuid.NewString()
	auditorsJSON := []byte("[]")
	if auditors := src.GetAuditors(); len(auditors) > 0 {
		type auditorRow struct {
			ID        string `json:"id"`
			Category  string `json:"category"`
			Status    string `json:"status"`
			Error     string `json:"error,omitempty"`
			ElapsedMs uint64 `json:"elapsed_ms"`
			LeakCount uint32 `json:"leak_count"`
		}
		rows := make([]auditorRow, 0, len(auditors))
		for _, a := range auditors {
			rows = append(rows, auditorRow{
				ID:        a.GetId(),
				Category:  a.GetCategory(),
				Status:    a.GetStatus(),
				Error:     a.GetError(),
				ElapsedMs: a.GetElapsedMs(),
				LeakCount: a.GetLeakCount(),
			})
		}
		if b, err := json.Marshal(rows); err == nil {
			auditorsJSON = b
		}
	}
	audit := &storage.ConfigAudit{
		ID:            auditID,
		ProjectID:     projectID,
		HostID:        hostID,
		StartedAtUnix: src.GetStartedAtUnix(),
		ElapsedMs:     int64(src.GetElapsedMs()),
		Error:         src.GetError(),
		AuditorsJSON:  string(auditorsJSON),
	}
	leaks := make([]*storage.ConfigLeak, 0, len(src.GetLeaks()))
	for _, l := range src.GetLeaks() {
		refsJSON := "[]"
		if refs := l.GetReferences(); len(refs) > 0 {
			if b, err := json.Marshal(refs); err == nil {
				refsJSON = string(b)
			}
		}
		leaks = append(leaks, &storage.ConfigLeak{
			ID:             uuid.NewString(),
			AuditID:        auditID,
			HostID:         hostID,
			ProjectID:      projectID,
			LeakID:         l.GetId(),
			AuditorID:      l.GetAuditorId(),
			Category:       l.GetCategory(),
			Risk:           l.GetRisk(),
			Title:          l.GetTitle(),
			Location:       l.GetLocation(),
			MatchRedacted:  l.GetMatch(),
			Pattern:        l.GetPattern(),
			Description:    l.GetDescription(),
			Remediation:    l.GetRemediation(),
			ReferencesJSON: refsJSON,
		})
	}
	return audit, leaks
}

// mergePartialAudit folds a partial-audit result (only the targeted
// auditors ran) into the prior persisted audit so the new row carries
// every auditor's most-recent state. Same shape as mergePartialScan
// — see that function for the rationale.
func mergePartialAudit(
	freshAudit *storage.ConfigAudit,
	freshLeaks []*storage.ConfigLeak,
	priorAudit *storage.ConfigAudit,
	priorLeaks []*storage.ConfigLeak,
	respProto *v2pb.ConfigAuditResponse,
) (*storage.ConfigAudit, []*storage.ConfigLeak) {
	if priorAudit == nil {
		return freshAudit, freshLeaks
	}
	targeted := make(map[string]struct{}, len(respProto.GetAuditors()))
	for _, a := range respProto.GetAuditors() {
		targeted[a.GetId()] = struct{}{}
	}

	merged := make([]*storage.ConfigLeak, 0, len(freshLeaks)+len(priorLeaks))
	merged = append(merged, freshLeaks...)
	for _, l := range priorLeaks {
		if _, hit := targeted[l.AuditorID]; hit {
			continue
		}
		clone := *l
		clone.ID = uuid.NewString()
		clone.AuditID = freshAudit.ID
		clone.ScannedAtUnix = freshAudit.StartedAtUnix
		merged = append(merged, &clone)
	}

	type auditorRow struct {
		ID        string `json:"id"`
		Category  string `json:"category"`
		Status    string `json:"status"`
		Error     string `json:"error,omitempty"`
		ElapsedMs uint64 `json:"elapsed_ms"`
		LeakCount uint32 `json:"leak_count"`
	}
	mergedAuditors := []auditorRow{}
	if len(freshAudit.AuditorsJSON) > 0 {
		_ = json.Unmarshal([]byte(freshAudit.AuditorsJSON), &mergedAuditors)
	}
	if priorAudit.AuditorsJSON != "" {
		var prior []auditorRow
		if err := json.Unmarshal([]byte(priorAudit.AuditorsJSON), &prior); err == nil {
			for _, p := range prior {
				if _, hit := targeted[p.ID]; hit {
					continue
				}
				mergedAuditors = append(mergedAuditors, p)
			}
		}
	}
	if b, err := json.Marshal(mergedAuditors); err == nil {
		freshAudit.AuditorsJSON = string(b)
	}
	return freshAudit, merged
}

// RegisterV1ConfigAuditRoutes mounts the per-host config-audit routes.
// Same role gates as the security routes: viewer reads, operator
// writes (kicks the agent + writes a DB row).
func RegisterV1ConfigAuditRoutes(engine *gin.Engine, h *HostsHandler, rbac *RBAC) {
	viewer := engine.Group("/api/v1/projects/:pid/hosts")
	viewer.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleViewer))
	{
		viewer.GET("/:hid/config-audit", h.GetConfigAudit)
		viewer.GET("/:hid/config-audits", h.ListConfigAudits)
		viewer.GET("/:hid/config-auditors", h.ListAvailableAuditors)
	}

	operator := engine.Group("/api/v1/projects/:pid/hosts")
	operator.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleOperator))
	{
		operator.POST("/:hid/config-audit", h.ReauditHost)
	}
}
