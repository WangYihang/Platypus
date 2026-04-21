package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// LiveListeners is the runtime-side abstraction for binding TCP ports
// and tearing them down. The production implementation is backed by
// core.TCPServer; tests substitute a fake that records calls without
// touching the network.
type LiveListeners interface {
	Create(host string, port uint16, projectID string) (id string, err error)
	Delete(id string) error
}

// ListenersV2Handler serves /projects/:pid/listeners[...]. It coordinates
// two sides that must stay in sync: the live TCP server managed by
// core (bound port, agent acceptance) and the persistent DB row the UI
// can still see after a restart.
//
// Naming: "V2" to distinguish from the legacy flat /listeners endpoints
// in handler_listeners_v1.go, which stay alive during transition.
type ListenersV2Handler struct {
	db   *storage.DB
	live LiveListeners
}

func NewListenersV2Handler(db *storage.DB, live LiveListeners) *ListenersV2Handler {
	return &ListenersV2Handler{db: db, live: live}
}

type createListenerV2Request struct {
	Host string `json:"host" binding:"required"`
	Port uint16 `json:"port" binding:"required"`
}

type listenerV2Response struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Host      string    `json:"host"`
	Port      uint16    `json:"port"`
	PublicIP  string    `json:"public_ip,omitempty"`
	ShellPath string    `json:"shell_path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func toListenerV2Response(l *storage.Listener) listenerV2Response {
	return listenerV2Response{
		ID: l.ID, ProjectID: l.ProjectID,
		Host: l.Host, Port: l.Port,
		PublicIP: l.PublicIP, ShellPath: l.ShellPath,
		CreatedAt: l.CreatedAt,
	}
}

// Create binds the port via live, then persists the row. If binding
// fails we never write to the DB — the UI sees nothing, matching the
// runtime reality. The opposite ordering (persist-first) would leave
// zombie rows if the bind always fails (wrong port, privilege denied).
func (h *ListenersV2Handler) Create(c *gin.Context) {
	var req createListenerV2Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	projectID := c.Param("pid")

	id, err := h.live.Create(req.Host, req.Port, projectID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "bind: " + err.Error()})
		return
	}
	row := &storage.Listener{
		ID:        id,
		ProjectID: projectID,
		Host:      req.Host,
		Port:      req.Port,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.db.Listeners().Create(c.Request.Context(), row); err != nil {
		// Best-effort rollback so live/persistent state don't drift.
		_ = h.live.Delete(id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "persist listener"})
		return
	}
	core.BroadcastNotify(core.EventListenerCreated, map[string]any{
		"project_id":  row.ProjectID,
		"listener_id": row.ID,
		"host":        row.Host,
		"port":        row.Port,
	})
	c.JSON(http.StatusCreated, toListenerV2Response(row))
}

// List returns every persisted listener for the project. Run-only
// listeners that predate persistence (legacy flow) aren't returned here
// — they remain visible via the old /listeners endpoint.
func (h *ListenersV2Handler) List(c *gin.Context) {
	rows, err := h.db.Listeners().ListByProject(c.Request.Context(), c.Param("pid"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list listeners"})
		return
	}
	out := make([]listenerV2Response, 0, len(rows))
	for _, l := range rows {
		out = append(out, toListenerV2Response(l))
	}
	c.JSON(http.StatusOK, gin.H{"listeners": out})
}

// Delete stops the live listener and removes the persisted row. Errors
// on live deletion are not fatal — we still drop the DB row so the UI
// stays consistent with the "no listener here" state the user intended.
func (h *ListenersV2Handler) Delete(c *gin.Context) {
	id := c.Param("lid")
	pid := c.Param("pid")

	// Cross-project isolation: a listener from another project must not
	// be deletable via this URL.
	l, err := h.db.Listeners().GetByID(c.Request.Context(), id)
	if errors.Is(err, storage.ErrNotFound) || (err == nil && l.ProjectID != pid) {
		c.JSON(http.StatusNotFound, gin.H{"error": "listener not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup listener"})
		return
	}

	_ = h.live.Delete(id) // non-fatal
	if err := h.db.Listeners().Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete listener"})
		return
	}
	core.BroadcastNotify(core.EventListenerDeleted, map[string]any{
		"project_id":  pid,
		"listener_id": id,
	})
	c.Status(http.StatusNoContent)
}

// RegisterV1ProjectListenersRoutes mounts the /projects/:pid/listeners
// surface. Viewer for reads, operator for writes.
func RegisterV1ProjectListenersRoutes(engine *gin.Engine, h *ListenersV2Handler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/listeners")
	grp.Use(rbac.RequireAuth())
	{
		grp.GET("",
			rbac.RequireProjectRole("pid", user.RoleViewer), h.List)
		grp.POST("",
			rbac.RequireProjectRole("pid", user.RoleOperator), h.Create)
		grp.DELETE("/:lid",
			rbac.RequireProjectRole("pid", user.RoleOperator), h.Delete)
	}
}
