package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/log"
	"github.com/WangYihang/Platypus/internal/recording"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// RecordingsHandler exposes the per-project terminal recording APIs.
// The cast files live on disk under recMgr.Dir(); the DB row carries
// summary metadata (size, duration, status) that the UI list page
// uses to render cards without hitting the filesystem.
type RecordingsHandler struct {
	db     *storage.DB
	recMgr *recording.Manager
}

func NewRecordingsHandler(db *storage.DB, recMgr *recording.Manager) *RecordingsHandler {
	return &RecordingsHandler{db: db, recMgr: recMgr}
}

// recordingResponse is the wire shape for a single row. Username +
// host_alias are looked up so the UI can render readable cards
// without fan-out fetches per item.
type recordingResponse struct {
	ID           string     `json:"id"`
	ProjectID    string     `json:"project_id"`
	HostID       string     `json:"host_id"`
	HostAlias    string     `json:"host_alias,omitempty"`
	HostHostname string     `json:"host_hostname,omitempty"`
	AgentID      string     `json:"agent_id,omitempty"`
	UserID       string     `json:"user_id,omitempty"`
	Username     string     `json:"username,omitempty"`
	Cols         int        `json:"cols"`
	Rows         int        `json:"rows"`
	Shell        string     `json:"shell,omitempty"`
	Title        string     `json:"title,omitempty"`
	SizeBytes    int64      `json:"size_bytes"`
	DurationMs   int64      `json:"duration_ms"`
	FrameCount   int64      `json:"frame_count"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"error_message,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
}

func (h *RecordingsHandler) toResponse(c *gin.Context, r *storage.TerminalRecording, hostCache map[string]hostBrief, userCache map[string]string) recordingResponse {
	out := recordingResponse{
		ID:           r.ID,
		ProjectID:    r.ProjectID,
		HostID:       r.HostID,
		AgentID:      r.AgentID,
		UserID:       r.UserID,
		Cols:         r.Cols,
		Rows:         r.Rows,
		Shell:        r.Shell,
		Title:        r.Title,
		SizeBytes:    r.SizeBytes,
		DurationMs:   r.DurationMs,
		FrameCount:   r.FrameCount,
		Status:       r.Status,
		ErrorMessage: r.ErrorMessage,
		StartedAt:    r.StartedAt,
		EndedAt:      r.EndedAt,
	}
	if r.HostID != "" {
		if cached, ok := hostCache[r.HostID]; ok {
			out.HostAlias = cached.alias
			out.HostHostname = cached.hostname
		} else {
			host, err := h.db.Hosts().GetByID(c.Request.Context(), r.HostID)
			if err == nil {
				cached := hostBrief{alias: host.PrimaryAlias, hostname: host.Hostname}
				hostCache[r.HostID] = cached
				out.HostAlias = cached.alias
				out.HostHostname = cached.hostname
			}
		}
	}
	if r.UserID != "" {
		if cached, ok := userCache[r.UserID]; ok {
			out.Username = cached
		} else {
			u, err := h.db.Users().GetByID(c.Request.Context(), r.UserID)
			if err == nil {
				userCache[r.UserID] = u.Username
				out.Username = u.Username
			}
		}
	}
	return out
}

type hostBrief struct {
	alias    string
	hostname string
}

// List handles GET /projects/:pid/recordings. Cursor-based pagination:
// next_cursor is the started_at of the last item; pass it back as
// ?cursor= to fetch the next page.
//
// Optional query params: ?host_id=, ?user_id=, ?status=, ?q= (free
// text), ?limit= (default 24, capped at 200).
func (h *RecordingsHandler) List(c *gin.Context) {
	pid := c.Param("pid")

	limit := 24
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 200 {
				n = 200
			}
			limit = n
		}
	}

	filter := storage.RecordingFilter{
		ProjectID: pid,
		HostID:    c.Query("host_id"),
		UserID:    c.Query("user_id"),
		AgentID:   c.Query("agent_id"),
		Status:    c.Query("status"),
		Q:         c.Query("q"),
		Limit:     limit + 1, // fetch one extra to detect "has next"
	}
	if cur := c.Query("cursor"); cur != "" {
		t, err := time.Parse(time.RFC3339Nano, cur)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cursor must be RFC3339Nano"})
			return
		}
		filter.Cursor = &t
	}

	rows, err := h.db.TerminalRecordings().List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list recordings"})
		return
	}

	var nextCursor string
	if len(rows) > limit {
		// Item at index `limit` is the next page's first row; use the
		// previous row's started_at as the cursor.
		last := rows[limit-1]
		nextCursor = last.StartedAt.UTC().Format(time.RFC3339Nano)
		rows = rows[:limit]
	}

	hostCache := make(map[string]hostBrief, len(rows))
	userCache := make(map[string]string, len(rows))
	out := make([]recordingResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, h.toResponse(c, r, hostCache, userCache))
	}

	// total is the unfiltered-by-cursor count for the same filter, so
	// the UI can render "X recordings" without paging through.
	totalFilter := filter
	totalFilter.Cursor = nil
	totalFilter.Limit = -1
	total, _ := h.db.TerminalRecordings().Count(c.Request.Context(), totalFilter)

	resp := gin.H{
		"items": out,
		"total": total,
	}
	if nextCursor != "" {
		resp["next_cursor"] = nextCursor
	}
	c.JSON(http.StatusOK, resp)
}

// Get handles GET /projects/:pid/recordings/:id. Returns the row
// metadata; use Download to stream the actual cast bytes.
func (h *RecordingsHandler) Get(c *gin.Context) {
	rec, err := h.lookup(c)
	if err != nil {
		return
	}
	hostCache := make(map[string]hostBrief, 1)
	userCache := make(map[string]string, 1)
	c.JSON(http.StatusOK, h.toResponse(c, rec, hostCache, userCache))
}

// Download handles GET /projects/:pid/recordings/:id/cast. Streams the
// raw asciinema v2 cast file with text/plain content-type — players
// like asciinema-player consume it as a UTF-8 stream of newline-
// delimited JSON. Browser-side <a download> works.
func (h *RecordingsHandler) Download(c *gin.Context) {
	rec, err := h.lookup(c)
	if err != nil {
		return
	}
	if rec.Status == storage.RecordingStatusRecording {
		c.JSON(http.StatusConflict, gin.H{"error": "recording still in progress"})
		return
	}

	if h.recMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "recording disabled"})
		return
	}
	path := h.recMgr.PathFor(rec)
	if path == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording file unavailable"})
		return
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusGone, gin.H{"error": "recording file removed"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "open recording"})
		return
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "stat recording"})
		return
	}

	filename := fmt.Sprintf("recording-%s.cast", rec.ID)
	if c.Query("download") == "1" {
		c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	} else {
		c.Header("Content-Disposition", `inline; filename="`+filename+`"`)
	}
	// asciinema v2 is JSON Lines; advertise it as text. Players that
	// fetch via Range still work because http.ServeContent honours it.
	c.Header("Content-Type", "application/x-asciicast; charset=utf-8")
	c.Header("Cache-Control", "private, max-age=86400")
	http.ServeContent(c.Writer, c.Request, filename, stat.ModTime(), f)
}

// updateRequest is the body for PATCH /projects/:pid/recordings/:id.
// Only Title is editable today.
type updateRecordingRequest struct {
	Title *string `json:"title"`
}

// Update handles PATCH /projects/:pid/recordings/:id. Currently only
// the operator-editable title is mutable.
func (h *RecordingsHandler) Update(c *gin.Context) {
	rec, err := h.lookup(c)
	if err != nil {
		return
	}
	var req updateRecordingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if len(title) > 200 {
			title = title[:200]
		}
		if err := h.db.TerminalRecordings().SetTitle(c.Request.Context(), rec.ID, title); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update recording"})
			return
		}
		rec.Title = title
	}

	RecordActivity(c, ActivityInput{
		Category:   storage.CategorySession,
		Action:     "recording.update",
		TargetType: "recording",
		TargetID:   rec.ID,
		At:         time.Now().UTC(),
	})

	hostCache := make(map[string]hostBrief, 1)
	userCache := make(map[string]string, 1)
	c.JSON(http.StatusOK, h.toResponse(c, rec, hostCache, userCache))
}

// Delete handles DELETE /projects/:pid/recordings/:id. Removes the DB
// row and the on-disk cast file. Best-effort on the file: a missing
// or unreadable file is ignored so a botched recording can still be
// cleaned out of the list.
func (h *RecordingsHandler) Delete(c *gin.Context) {
	rec, err := h.lookup(c)
	if err != nil {
		return
	}

	if err := h.db.TerminalRecordings().Delete(c.Request.Context(), rec.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete recording"})
		return
	}
	if h.recMgr != nil {
		if err := h.recMgr.DeleteFile(rec); err != nil {
			log.L.Warn("recording_file_unlink_failed",
				"id", rec.ID,
				"error", err.Error(),
			)
		}
	}

	RecordActivity(c, ActivityInput{
		Category:   storage.CategorySession,
		Action:     "recording.delete",
		TargetType: "recording",
		TargetID:   rec.ID,
		At:         time.Now().UTC(),
	})

	c.Status(http.StatusNoContent)
}

// lookup loads a row by :id and verifies it belongs to :pid. On
// failure it writes the response and returns a non-nil error so the
// caller can short-circuit.
func (h *RecordingsHandler) lookup(c *gin.Context) (*storage.TerminalRecording, error) {
	pid := c.Param("pid")
	id := c.Param("id")
	rec, err := h.db.TerminalRecordings().Get(c.Request.Context(), id)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return nil, err
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup recording"})
		return nil, err
	}
	if rec.ProjectID != pid {
		// Don't leak existence across projects.
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return nil, errors.New("project mismatch")
	}
	return rec, nil
}

// RegisterV1RecordingRoutes mounts the per-project recording routes.
// List/Get/Download are viewer-tier (everyone in the project can audit
// their own and others' sessions); Update/Delete are operator-tier
// because mutating audit records is privileged.
func RegisterV1RecordingRoutes(engine *gin.Engine, h *RecordingsHandler, rbac *RBAC) {
	g := engine.Group("/api/v1/projects/:pid/recordings")
	g.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleViewer))
	{
		g.GET("", h.List)
		g.GET("/:id", h.Get)
		g.GET("/:id/cast", h.Download)
	}
	op := engine.Group("/api/v1/projects/:pid/recordings")
	op.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleOperator))
	{
		op.PATCH("/:id", h.Update)
		op.DELETE("/:id", h.Delete)
	}
}
