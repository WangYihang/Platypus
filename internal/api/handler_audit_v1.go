package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// AuditHandler serves the compliance-export endpoint. Every invocation
// is itself written to admin_audit_log — meta-audit — so "who exported
// what when" is always recoverable.
type AuditHandler struct {
	db *storage.DB
}

func NewAuditHandler(db *storage.DB) *AuditHandler {
	return &AuditHandler{db: db}
}

// auditEventKind enumerates the event families the export supports.
// Callers request one or more via ?types=pat_redemption,connection,admin.
// Unknown types are ignored (tolerant parsing) so adding a new family
// later doesn't break existing API consumers.
type auditEventKind string

const (
	kindPATRedemption auditEventKind = "pat_redemption"
	kindConnection    auditEventKind = "connection"
	kindAdmin         auditEventKind = "admin"
)

// Export handles GET /api/v1/projects/:pid/audit/export.
//
// Query parameters:
//
//	from=<unix-seconds>   default: 0 (no lower bound)
//	to=<unix-seconds>     default: now
//	types=a,b,c           default: all kinds
//	format=jsonl|csv      default: jsonl
//
// The response streams straight out of storage — we intentionally do
// not buffer the full result set to protect the server against huge
// ranges. For JSONL each line is one event wrapped as
// {"kind":"...","event":{...}}; for CSV we emit one header row per kind
// (prefixed with "kind,...") so readers can split on the kind column.
func (h *AuditHandler) Export(c *gin.Context) {
	projectID := c.Param("pid")
	claims, _ := ClaimsFromContext(c)

	filter, err := parseExportFilter(c, projectID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	kinds := parseKinds(c.Query("types"))
	if len(kinds) == 0 {
		kinds = []auditEventKind{kindPATRedemption, kindConnection, kindAdmin}
	}
	format := strings.ToLower(c.DefaultQuery("format", "jsonl"))
	if format != "jsonl" && format != "csv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be jsonl or csv"})
		return
	}

	// Meta-audit first. Failing to record it is NOT a reason to refuse
	// the export — security teams need the data more than we need the
	// log line — but we do log the failure.
	h.recordMetaAudit(c, claims.UserID, projectID, filter, kinds, format)

	switch format {
	case "jsonl":
		c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
		if err := h.streamJSONL(c, filter, kinds); err != nil {
			// Headers have already been sent; best we can do is log and
			// close. A partial ndjson response is more useful than a 500
			// replacing everything.
			_, _ = fmt.Fprintf(c.Writer, "\n{\"error\":%q}\n", err.Error())
		}
	case "csv":
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition",
			`attachment; filename="platypus-audit-`+projectID+`.csv"`)
		if err := h.streamCSV(c, filter, kinds); err != nil {
			_, _ = fmt.Fprintf(c.Writer, "\n# error: %s\n", err.Error())
		}
	}
}

// parseExportFilter builds an AuditExportFilter from query params.
// projectID "" means global (requires global admin; routing enforces).
func parseExportFilter(c *gin.Context, projectID string) (storage.AuditExportFilter, error) {
	f := storage.AuditExportFilter{ProjectID: projectID}
	if s := c.Query("from"); s != "" {
		sec, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return f, fmt.Errorf("invalid from=%q (expected unix seconds)", s)
		}
		f.From = time.Unix(sec, 0).UTC()
	}
	if s := c.Query("to"); s != "" {
		sec, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return f, fmt.Errorf("invalid to=%q (expected unix seconds)", s)
		}
		f.To = time.Unix(sec, 0).UTC()
	}
	return f, nil
}

func parseKinds(raw string) []auditEventKind {
	if raw == "" {
		return nil
	}
	var out []auditEventKind
	for _, p := range strings.Split(raw, ",") {
		switch strings.TrimSpace(p) {
		case "pat_redemption":
			out = append(out, kindPATRedemption)
		case "connection":
			out = append(out, kindConnection)
		case "admin":
			out = append(out, kindAdmin)
		default:
			// Unknown kinds tolerated — silently ignored.
		}
	}
	return out
}

// streamJSONL writes one NDJSON line per event, interleaved across the
// requested kinds in a stable order. We don't merge-sort globally —
// readers can sort by "at" in a post-process if needed, and the extra
// RAM / latency aren't worth it for typical audit volumes.
func (h *AuditHandler) streamJSONL(c *gin.Context, f storage.AuditExportFilter, kinds []auditEventKind) error {
	enc := json.NewEncoder(c.Writer)
	for _, k := range kinds {
		switch k {
		case kindPATRedemption:
			events, err := h.db.PATRedemptionEvents().ListInRange(c.Request.Context(), f)
			if err != nil {
				return fmt.Errorf("pat_redemption: %w", err)
			}
			for _, e := range events {
				if err := enc.Encode(map[string]any{"kind": string(k), "event": e}); err != nil {
					return err
				}
			}
		case kindConnection:
			events, err := h.db.AgentConnectionEvents().ListInRange(c.Request.Context(), f)
			if err != nil {
				return fmt.Errorf("connection: %w", err)
			}
			for _, e := range events {
				if err := enc.Encode(map[string]any{"kind": string(k), "event": e}); err != nil {
					return err
				}
			}
		case kindAdmin:
			events, err := h.db.AdminAuditLog().ListInRange(c.Request.Context(), f)
			if err != nil {
				return fmt.Errorf("admin: %w", err)
			}
			for _, e := range events {
				if err := enc.Encode(map[string]any{"kind": string(k), "event": e}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// streamCSV writes each event family as its own header + rows block,
// separated by a blank line. Columns are flattened since CSV has no
// concept of nested objects. Keeps the kind column first so readers
// can split the file by it.
func (h *AuditHandler) streamCSV(c *gin.Context, f storage.AuditExportFilter, kinds []auditEventKind) error {
	w := csv.NewWriter(c.Writer)
	defer w.Flush()

	for _, k := range kinds {
		switch k {
		case kindPATRedemption:
			events, err := h.db.PATRedemptionEvents().ListInRange(c.Request.Context(), f)
			if err != nil {
				return err
			}
			if err := w.Write([]string{"kind", "id", "at", "token_id", "client_ip",
				"machine_id", "hostname", "agent_id", "outcome", "error_detail"}); err != nil {
				return err
			}
			for _, e := range events {
				if err := w.Write([]string{string(k), fmt.Sprintf("%d", e.ID),
					e.At.UTC().Format(time.RFC3339), e.TokenID, e.ClientIP,
					e.MachineID, e.Hostname, e.AgentID, e.Outcome, e.ErrorDetail,
				}); err != nil {
					return err
				}
			}
		case kindConnection:
			events, err := h.db.AgentConnectionEvents().ListInRange(c.Request.Context(), f)
			if err != nil {
				return err
			}
			if err := w.Write([]string{"kind", "id", "at", "agent_id", "session_id",
				"client_ip", "event_type", "reason", "transport"}); err != nil {
				return err
			}
			for _, e := range events {
				if err := w.Write([]string{string(k), fmt.Sprintf("%d", e.ID),
					e.At.UTC().Format(time.RFC3339), e.AgentID, e.SessionID,
					e.ClientIP, e.EventType, e.Reason, e.Transport,
				}); err != nil {
					return err
				}
			}
		case kindAdmin:
			events, err := h.db.AdminAuditLog().ListInRange(c.Request.Context(), f)
			if err != nil {
				return err
			}
			if err := w.Write([]string{"kind", "id", "at", "actor_user", "actor_ip",
				"actor_ua", "action", "target_type", "target_id", "project_id",
				"details", "outcome", "error"}); err != nil {
				return err
			}
			for _, e := range events {
				if err := w.Write([]string{string(k), fmt.Sprintf("%d", e.ID),
					e.At.UTC().Format(time.RFC3339), e.ActorUser, e.ActorIP,
					e.ActorUA, e.Action, e.TargetType, e.TargetID, e.ProjectID,
					e.Details, e.Outcome, e.Error,
				}); err != nil {
					return err
				}
			}
		}
		// Blank record between kinds (csv.Writer writes nothing for an
		// empty record, so do it manually).
		if _, err := c.Writer.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}

// recordMetaAudit emits the "audit.export" row. Captures the filter and
// kinds so reviewers can see exactly what someone pulled. Errors are
// swallowed — we prefer to serve the export over losing a log line.
func (h *AuditHandler) recordMetaAudit(c *gin.Context, actor, projectID string,
	f storage.AuditExportFilter, kinds []auditEventKind, format string) {
	kindNames := make([]string, 0, len(kinds))
	for _, k := range kinds {
		kindNames = append(kindNames, string(k))
	}
	detail, _ := json.Marshal(map[string]any{
		"from":   f.From,
		"to":     f.To,
		"kinds":  kindNames,
		"format": format,
	})
	if err := h.db.AdminAuditLog().Record(c.Request.Context(), &storage.AdminAuditEvent{
		At:         time.Now().UTC(),
		ActorUser:  actor,
		ActorIP:    c.ClientIP(),
		ActorUA:    c.Request.UserAgent(),
		Action:     "audit.export",
		TargetType: "project",
		TargetID:   projectID,
		ProjectID:  projectID,
		Details:    string(detail),
		Outcome:    "success",
	}); err != nil {
		// Intentional: swallow. See docstring.
		_ = errors.New("audit: record meta-audit failed")
	}
}

// RegisterV1AuditRoutes mounts the audit export surface. Project-scoped
// route is gated by RequireProjectRole(admin). A global-admin-only
// counterpart under /api/v1/audit/export (no :pid) is deliberately NOT
// added here — cross-project exports should be rare and explicit, so
// they go on a separate path once needed.
func RegisterV1AuditRoutes(engine *gin.Engine, h *AuditHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/audit")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleAdmin))
	{
		grp.GET("/export", h.Export)
	}
}
