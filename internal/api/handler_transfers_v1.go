package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// TransferRoutesDeps bundles the collaborators the transfers REST
// API needs. DB is read-only here (writes happen via the recorder
// in the archive handler); Cancels lets us tear down in-flight
// transfers.
type TransferRoutesDeps struct {
	DB      *storage.DB
	RBAC    *RBAC
	Cancels *TransferCancelRegistry
}

// RegisterV1TransferRoutes mounts:
//   GET  /api/v1/projects/:pid/transfers
//   GET  /api/v1/projects/:pid/hosts/:hid/transfers
//   POST /api/v1/projects/:pid/transfers/:id/cancel
//   GET  /api/v1/transfers              (admin only — global)
//
// All read endpoints are viewer-tier; cancel is operator-tier
// (mutating effect on an in-flight resource).
func RegisterV1TransferRoutes(engine *gin.Engine, deps TransferRoutesDeps) {
	if deps.Cancels == nil {
		deps.Cancels = NewTransferCancelRegistry()
	}
	auth := engine.Group("")
	auth.Use(deps.RBAC.RequireAuth())

	// Per-project + per-host listing.
	proj := auth.Group("/api/v1/projects/:pid")
	proj.Use(deps.RBAC.RequireProjectRole("pid", user.RoleViewer))
	proj.GET("/transfers", listTransfers(deps, false))
	proj.GET("/hosts/:hid/transfers", listTransfers(deps, true))

	// Cancellation requires operator-tier on the project.
	cancel := auth.Group("/api/v1/projects/:pid")
	cancel.Use(deps.RBAC.RequireProjectRole("pid", user.RoleOperator))
	cancel.POST("/transfers/:id/cancel", cancelTransfer(deps))

	// Global (admin-only) listing.
	admin := auth.Group("/api/v1")
	admin.Use(deps.RBAC.RequireGlobalRole(user.RoleAdmin))
	admin.GET("/transfers", listTransfers(deps, false))
}

// transferItem is the JSON shape surfaced via the REST API. The
// stored PathsJSON is parsed back into a string slice so frontends
// don't double-decode.
type transferItem struct {
	ID               string  `json:"id"`
	ProjectID        string  `json:"project_id"`
	HostID           string  `json:"host_id"`
	UserID           string  `json:"user_id"`
	Direction        string  `json:"direction"`
	Kind             string  `json:"kind"`
	Format           string  `json:"format"`
	Paths            []string `json:"paths"`
	Status           string  `json:"status"`
	BytesTransferred int64   `json:"bytes_transferred"`
	TotalBytes       int64   `json:"total_bytes"`
	ErrorMessage     string  `json:"error_message,omitempty"`
	StartedAt        string  `json:"started_at"`
	FinishedAt       string  `json:"finished_at,omitempty"`
}

func toTransferItem(ft *storage.FileTransfer) transferItem {
	out := transferItem{
		ID:               ft.ID,
		ProjectID:        ft.ProjectID,
		HostID:           ft.HostID,
		UserID:           ft.UserID,
		Direction:        ft.Direction,
		Kind:             ft.Kind,
		Format:           ft.Format,
		Status:           ft.Status,
		BytesTransferred: ft.BytesTransferred,
		TotalBytes:       ft.TotalBytes,
		ErrorMessage:     ft.ErrorMessage,
		StartedAt:        ft.StartedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
	}
	if ft.FinishedAt != nil {
		out.FinishedAt = ft.FinishedAt.UTC().Format("2006-01-02T15:04:05.000000000Z")
	}
	out.Paths = parsePathsJSON(ft.PathsJSON)
	return out
}

func listTransfers(deps TransferRoutesDeps, perHost bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		filter := storage.FileTransferFilter{
			ProjectID: c.Param("pid"),
		}
		if perHost {
			filter.HostID = c.Param("hid")
		} else if hid := c.Query("host_id"); hid != "" {
			filter.HostID = hid
		}
		if status := c.Query("status"); status != "" {
			filter.Status = status
		}
		if limitStr := c.Query("limit"); limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				filter.Limit = n
			}
		}
		rows, err := deps.DB.FileTransfers().List(c.Request.Context(), filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out := make([]transferItem, 0, len(rows))
		for _, ft := range rows {
			out = append(out, toTransferItem(ft))
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
	}
}

func cancelTransfer(deps TransferRoutesDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.String(http.StatusBadRequest, "id required")
			return
		}
		// Sanity check: the transfer must exist for THIS project so
		// callers can't cancel a transfer from another project they
		// happen to know the ID of. (Cancellation alone is harmless,
		// but we don't want to leak the existence of the row.)
		ft, err := deps.DB.FileTransfers().Get(c.Request.Context(), id)
		if err != nil || ft.ProjectID != c.Param("pid") {
			c.JSON(http.StatusNotFound, gin.H{"error": "transfer not found"})
			return
		}
		if !deps.Cancels.Cancel(id) {
			// Row exists but the in-flight goroutine is gone — most
			// commonly the transfer already finished. Treat that as
			// 404 too: nothing to cancel.
			c.JSON(http.StatusNotFound, gin.H{"error": "no in-flight transfer to cancel"})
			return
		}
		c.Status(http.StatusAccepted)
	}
}

// parsePathsJSON unmarshals the stored JSON array, returning an
// empty slice on error so the JSON output stays well-formed.
func parsePathsJSON(s string) []string {
	if s == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}
