package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// ActivitiesHandler serves read/export over the unified activity log.
// Writes always go via ActivityRecorder, never this handler.
type ActivitiesHandler struct {
	db *storage.DB
}

// NewActivitiesHandler builds a handler bound to the given storage.
func NewActivitiesHandler(db *storage.DB) *ActivitiesHandler {
	return &ActivitiesHandler{db: db}
}

// activityItem is the JSON shape surfaced to API clients. Differs from
// storage.Activity in one place: Meta is parsed from the stored JSON
// string into a free-form map so frontends don't have to double-decode.
type activityItem struct {
	ID           int64     `json:"id"`
	At           time.Time `json:"at"`
	ProjectID    *string   `json:"project_id"`
	ActorType    string    `json:"actor_type"`
	ActorUser    string    `json:"actor_user,omitempty"`
	ActorIP      string    `json:"actor_ip,omitempty"`
	ActorUA      string    `json:"actor_ua,omitempty"`
	ActorTokenID string    `json:"actor_token_id,omitempty"`
	Category     string    `json:"category"`
	Action       string    `json:"action"`
	TargetType   string    `json:"target_type,omitempty"`
	TargetID     string    `json:"target_id,omitempty"`
	TargetLabel  string    `json:"target_label,omitempty"`
	Outcome      string    `json:"outcome"`
	Error        string    `json:"error,omitempty"`
	DurationMs   *int64    `json:"duration_ms,omitempty"`
	RequestID    string    `json:"request_id,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	Meta         any       `json:"meta,omitempty"`
}

type listActivitiesResponse struct {
	Items      []activityItem `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
	Total      *int64         `json:"total,omitempty"`
}

// ListProject handles GET /api/v1/projects/:pid/activities.
//
// Query parameters:
//
//	from / to       RFC3339 timestamps (inclusive)
//	category        comma-separated list
//	action          repeated
//	actor           actor_user exact match
//	actor_type      comma-separated list of raw actor types
//	                (user|api_token|agent|system|anonymous)
//	source          high-level alias mapping to actor_type sets:
//	                  human  → user, api_token
//	                  agent  → agent
//	                  system → system, anonymous
//	                Multiple values may be comma-separated; combined
//	                with actor_type they union. Powers the
//	                "Users / Agents / System" segment in the UI.
//	outcome         success|denied|error
//	session_id      exact match on session_id
//	target_type     exact match
//	target_id       exact match
//	q               free-text LIKE on action / target_label / meta
//	limit           default 50, max 200
//	cursor          opaque keyset cursor
//	include_global  when true, merge project_id IS NULL rows (global)
//	include_total   when true, include Total count (extra query, opt-in)
func (h *ActivitiesHandler) ListProject(c *gin.Context) {
	projectID := c.Param("pid")
	filter, err := parseActivityListQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter.ProjectID = &projectID
	filter.IncludeGlobal = queryBool(c, "include_global")
	h.serveList(c, filter)
}

// ListGlobal handles GET /api/v1/activities. Surface-level auth is
// enforced at the router (global admin only).
func (h *ActivitiesHandler) ListGlobal(c *gin.Context) {
	filter, err := parseActivityListQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// No ProjectID filter → every row. A caller who wants only global
	// rows passes ?scope=global.
	switch strings.ToLower(c.Query("scope")) {
	case "global":
		empty := ""
		filter.ProjectID = &empty
	case "project":
		if pid := c.Query("project_id"); pid != "" {
			filter.ProjectID = &pid
		}
	}
	h.serveList(c, filter)
}

func (h *ActivitiesHandler) serveList(c *gin.Context, filter storage.ActivityFilter) {
	items, cursor, err := h.db.Activities().List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := listActivitiesResponse{
		Items:      toActivityItems(items),
		NextCursor: cursor,
	}
	if queryBool(c, "include_total") {
		total, err := h.db.Activities().Count(c.Request.Context(), filter)
		if err == nil {
			resp.Total = &total
		}
	}
	c.JSON(http.StatusOK, resp)
}

// ExportProject streams every matching activity as JSONL or CSV. No
// cursor-based pagination here — export is expected to be a one-shot,
// download-to-disk operation. The export itself is recorded as an
// activity (meta-audit).
func (h *ActivitiesHandler) ExportProject(c *gin.Context) {
	projectID := c.Param("pid")
	filter, err := parseActivityListQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter.ProjectID = &projectID
	filter.IncludeGlobal = queryBool(c, "include_global")
	h.stream(c, filter, projectID)
}

// ExportGlobal is the cross-project export; restricted to global admin.
func (h *ActivitiesHandler) ExportGlobal(c *gin.Context) {
	filter, err := parseActivityListQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.stream(c, filter, "")
}

func (h *ActivitiesHandler) stream(c *gin.Context, filter storage.ActivityFilter, projectID string) {
	format := strings.ToLower(c.DefaultQuery("format", "jsonl"))
	if format != "jsonl" && format != "csv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be jsonl or csv"})
		return
	}
	// Record the export itself first (best-effort, fire-and-forget).
	recordExportAudit(c, projectID, filter, format)

	// Paginate through storage to avoid holding the whole result set in
	// memory. Page size is the max-list limit (200); cursors are opaque
	// and self-contained so we can just loop until exhausted.
	filter.Limit = storage.MaxActivityListLimit

	switch format {
	case "jsonl":
		c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
		h.streamJSONL(c, filter)
	case "csv":
		fname := "platypus-activities"
		if projectID != "" {
			fname += "-" + projectID
		}
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="`+fname+`.csv"`)
		h.streamCSV(c, filter)
	}
}

func (h *ActivitiesHandler) streamJSONL(c *gin.Context, filter storage.ActivityFilter) {
	enc := json.NewEncoder(c.Writer)
	for {
		page, cursor, err := h.db.Activities().List(c.Request.Context(), filter)
		if err != nil {
			_, _ = fmt.Fprintf(c.Writer,
				"\n{\"error\":%q}\n", err.Error())
			return
		}
		for _, a := range page {
			if err := enc.Encode(toActivityItem(a)); err != nil {
				return
			}
		}
		if cursor == "" {
			return
		}
		filter.Cursor = cursor
	}
}

func (h *ActivitiesHandler) streamCSV(c *gin.Context, filter storage.ActivityFilter) {
	w := csv.NewWriter(c.Writer)
	defer w.Flush()

	if err := w.Write([]string{
		"id", "at", "project_id", "actor_type", "actor_user", "actor_ip",
		"actor_ua", "actor_token_id", "category", "action",
		"target_type", "target_id", "target_label", "outcome", "error",
		"duration_ms", "request_id", "session_id", "meta",
	}); err != nil {
		return
	}
	for {
		page, cursor, err := h.db.Activities().List(c.Request.Context(), filter)
		if err != nil {
			_, _ = fmt.Fprintf(c.Writer,
				"\n# error: %s\n", err.Error())
			return
		}
		for _, a := range page {
			projectID := ""
			if a.ProjectID != nil {
				projectID = *a.ProjectID
			}
			duration := ""
			if a.DurationMs != nil {
				duration = strconv.FormatInt(*a.DurationMs, 10)
			}
			if err := w.Write([]string{
				strconv.FormatInt(a.ID, 10),
				a.At.UTC().Format(time.RFC3339Nano),
				projectID, a.ActorType, a.ActorUser, a.ActorIP, a.ActorUA,
				a.ActorTokenID, a.Category, a.Action, a.TargetType,
				a.TargetID, a.TargetLabel, a.Outcome, a.Error,
				duration, a.RequestID, a.SessionID, a.Meta,
			}); err != nil {
				return
			}
		}
		w.Flush()
		if cursor == "" {
			return
		}
		filter.Cursor = cursor
	}
}

// parseActivityListQuery extracts filter parameters from the request.
// Returns an error on malformed timestamps or numeric fields; the rest
// are tolerated (unknown values just don't match).
func parseActivityListQuery(c *gin.Context) (storage.ActivityFilter, error) {
	f := storage.ActivityFilter{
		ActorUser:  c.Query("actor"),
		Outcome:    c.Query("outcome"),
		SessionID:  c.Query("session_id"),
		TargetType: c.Query("target_type"),
		TargetID:   c.Query("target_id"),
		Search:     c.Query("q"),
		Cursor:     c.Query("cursor"),
	}
	if s := c.Query("from"); s != "" {
		t, err := parseTimeFlexible(s)
		if err != nil {
			return f, fmt.Errorf("invalid from=%q: %w", s, err)
		}
		f.From = t
	}
	if s := c.Query("to"); s != "" {
		t, err := parseTimeFlexible(s)
		if err != nil {
			return f, fmt.Errorf("invalid to=%q: %w", s, err)
		}
		f.To = t
	}
	if s := c.Query("category"); s != "" {
		for _, p := range strings.Split(s, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				f.Categories = append(f.Categories, p)
			}
		}
	}
	if actions := c.QueryArray("action"); len(actions) > 0 {
		for _, a := range actions {
			for _, p := range strings.Split(a, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					f.Actions = append(f.Actions, p)
				}
			}
		}
	}
	// actor_type and source merge into the same dimension. We
	// dedupe so a caller passing both ?source=human&actor_type=user
	// doesn't end up with `actor_type IN ('user','user')` and a
	// confusing query plan.
	seenActor := make(map[string]struct{})
	addActorType := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		if _, ok := seenActor[t]; ok {
			return
		}
		seenActor[t] = struct{}{}
		f.ActorTypes = append(f.ActorTypes, t)
	}
	if s := c.Query("actor_type"); s != "" {
		for _, p := range strings.Split(s, ",") {
			addActorType(p)
		}
	}
	if s := c.Query("source"); s != "" {
		for _, p := range strings.Split(s, ",") {
			for _, t := range expandSourceAlias(p) {
				addActorType(t)
			}
		}
	}
	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return f, fmt.Errorf("invalid limit=%q", s)
		}
		f.Limit = n
	}
	return f, nil
}

// expandSourceAlias maps a high-level source bucket to the raw
// actor_type values stored on the row. Unknown aliases yield nil so
// they're skipped silently — the caller-side behaviour matches an
// "all sources" query, which is the safer fallback than 400'ing on a
// typo. Buckets:
//
//   - human  → user, api_token   (anything a person initiated, directly
//     or via an automation token issued to them)
//   - agent  → agent             (link lifecycle and agent-side
//     handshake events)
//   - system → system, anonymous (server lifecycle, retention sweeps,
//     and pre-auth events whose origin couldn't be attributed to any
//     principal)
func expandSourceAlias(alias string) []string {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case "human", "users", "user":
		return []string{storage.ActorTypeUser, storage.ActorTypeAPIToken}
	case "agent", "agents", "link":
		return []string{storage.ActorTypeAgent}
	case "system":
		return []string{storage.ActorTypeSystem, storage.ActorTypeAnonymous}
	default:
		return nil
	}
}

// parseTimeFlexible accepts RFC3339 or unix seconds. RFC3339 is preferred
// for human-written URLs; unix seconds stays for legacy integrations.
func parseTimeFlexible(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if sec, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(sec, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("not RFC3339 or unix seconds")
}

func queryBool(c *gin.Context, key string) bool {
	switch strings.ToLower(c.Query(key)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func toActivityItems(xs []*storage.Activity) []activityItem {
	out := make([]activityItem, 0, len(xs))
	for _, a := range xs {
		out = append(out, toActivityItem(a))
	}
	return out
}

func toActivityItem(a *storage.Activity) activityItem {
	item := activityItem{
		ID:           a.ID,
		At:           a.At.UTC(),
		ProjectID:    a.ProjectID,
		ActorType:    a.ActorType,
		ActorUser:    a.ActorUser,
		ActorIP:      a.ActorIP,
		ActorUA:      a.ActorUA,
		ActorTokenID: a.ActorTokenID,
		Category:     a.Category,
		Action:       a.Action,
		TargetType:   a.TargetType,
		TargetID:     a.TargetID,
		TargetLabel:  a.TargetLabel,
		Outcome:      a.Outcome,
		Error:        a.Error,
		DurationMs:   a.DurationMs,
		RequestID:    a.RequestID,
		SessionID:    a.SessionID,
	}
	if a.Meta != "" {
		var decoded any
		if err := json.Unmarshal([]byte(a.Meta), &decoded); err == nil {
			item.Meta = decoded
		} else {
			// Keep the raw string so no data is lost, even on a meta we
			// somehow can't re-decode.
			item.Meta = a.Meta
		}
	}
	return item
}

// recordExportAudit writes a meta-activity describing the export. Best
// effort — the export serves regardless of audit success.
func recordExportAudit(c *gin.Context, projectID string, f storage.ActivityFilter, format string) {
	meta := map[string]any{
		"format":      format,
		"from":        nullableTime(f.From),
		"to":          nullableTime(f.To),
		"categories":  f.Categories,
		"actions":     f.Actions,
		"actor_types": f.ActorTypes,
		"actor":       f.ActorUser,
		"outcome":     f.Outcome,
		"session_id":  f.SessionID,
		"search":      f.Search,
	}
	targetID := projectID
	if projectID == "" {
		targetID = "all"
	}
	in := ActivityInput{
		Category:    storage.CategoryAdmin,
		Action:      "activities.export",
		TargetType:  "activities",
		TargetID:    targetID,
		TargetLabel: targetID,
		Meta:        meta,
	}
	if projectID == "" {
		// Global export → force ProjectID to nil (global event).
		empty := ""
		in.ProjectID = &empty
	}
	RecordActivity(c, in)
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// RegisterV1ActivitiesRoutes mounts the read/export surface. Project
// routes accept any Viewer (you can see what happened in your project);
// global routes require a global Admin.
func RegisterV1ActivitiesRoutes(engine *gin.Engine, h *ActivitiesHandler, rbac *RBAC) {
	proj := engine.Group("/api/v1/projects/:pid/activities")
	proj.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleViewer))
	{
		proj.GET("", h.ListProject)
		proj.GET("/export", h.ExportProject)
	}
	global := engine.Group("/api/v1/activities")
	global.Use(rbac.RequireAuth(), rbac.RequireGlobalRole(user.RoleAdmin))
	{
		global.GET("", h.ListGlobal)
		global.GET("/export", h.ExportGlobal)
	}
}
