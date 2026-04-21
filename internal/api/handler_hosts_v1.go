package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// HostsHandler serves per-project host aggregation routes. Host writes
// happen out-of-band (agent handshake) — this handler is read-only.
type HostsHandler struct {
	db *storage.DB
}

func NewHostsHandler(db *storage.DB) *HostsHandler {
	return &HostsHandler{db: db}
}

// hostResponse is the JSON shape of a Host on the wire. Keeps the internal
// HostIdentity fields out of the response and stamps booleans in the
// underscore_case expected by the frontend.
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

// RegisterV1HostsRoutes mounts the per-project host routes under
// /api/v1/projects/:pid/hosts. Every route is RequireAuth +
// RequireProjectRole(viewer).
func RegisterV1HostsRoutes(engine *gin.Engine, h *HostsHandler, rbac *RBAC) {
	grp := engine.Group("/api/v1/projects/:pid/hosts")
	grp.Use(rbac.RequireAuth(), rbac.RequireProjectRole("pid", user.RoleViewer))
	{
		grp.GET("", h.List)
		grp.GET("/:hid", h.Get)
	}
}
