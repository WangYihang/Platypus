package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// SecurityFindingsHandler serves the project-level cross-host
// findings list — the flat table that powers the Security top-tab
// in the UI. The per-host history endpoints live next to the host
// CRUD on HostsHandler.
type SecurityFindingsHandler struct {
	db *storage.DB
}

func NewSecurityFindingsHandler(db *storage.DB) *SecurityFindingsHandler {
	return &SecurityFindingsHandler{db: db}
}

// findingsPageResponse is the JSON shape for List. Mirrors the
// storage.Page envelope but with the response-shaped finding rows.
type findingsPageResponse struct {
	Findings []findingResponse `json:"findings"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// List handles GET /api/v1/projects/:pid/security-findings.
//
// Query params (all optional):
//
//	severity=critical,high   CSV — restrict to one or more severities
//	category=ssh,kernel      CSV — restrict to one or more categories
//	host_id=<uuid>           single — restrict to one host
//	q=<substring>            case-insensitive match on title + evidence
//	page=<n>                 1-indexed; default 1
//	page_size=<n>            default 50, capped at 200
//
// The storage layer restricts results to the latest scan per host so
// the project view always shows current posture (operators don't
// have to filter out historical noise from older scans).
func (h *SecurityFindingsHandler) List(c *gin.Context) {
	pid := c.Param("pid")
	filter := storage.ListFindingsFilter{
		HostID: c.Query("host_id"),
		Q:      c.Query("q"),
	}
	if s := c.Query("severity"); s != "" {
		filter.Severity = splitCSV(s)
	}
	if s := c.Query("category"); s != "" {
		filter.Category = splitCSV(s)
	}
	page := storage.Page{}
	if s := c.Query("page"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			page.Page = n
		}
	}
	if s := c.Query("page_size"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			page.PageSize = n
		}
	}

	findings, total, err := h.db.SecurityScans().ListFindings(c.Request.Context(), pid, filter, page)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list findings"})
		return
	}
	out := findingsPageResponse{
		Findings: make([]findingResponse, 0, len(findings)),
		Total:    total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}
	if out.Page < 1 {
		out.Page = 1
	}
	if out.PageSize <= 0 {
		out.PageSize = 50
	}
	for _, f := range findings {
		out.Findings = append(out.Findings, toFindingResponse(f, true))
	}
	c.JSON(http.StatusOK, out)
}

// RegisterV1SecurityFindingsRoutes mounts the project-scoped
// findings routes. Read-only — the writes happen via the per-host
// rescan endpoint on HostsHandler.
func RegisterV1SecurityFindingsRoutes(engine *gin.Engine, h *SecurityFindingsHandler, rbac *RBAC) {
	g := engine.Group("/api/v1/projects/:pid/security-findings")
	g.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleViewer))
	g.GET("", h.List)
}

// splitCSV splits "a,b,,c" into []string{"a","b","c"}, trimming
// whitespace and dropping empties. Used for the comma-separated
// query params.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
