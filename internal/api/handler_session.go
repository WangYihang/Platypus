package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
)

// patchSessionRequest is documented purely for swag — gin handlers decode into
// an anonymous struct. Keeping a typed mirror here gives OpenAPI a schema.
type patchSessionRequest struct {
	Alias         *string `json:"alias,omitempty"`
	GroupDispatch *bool   `json:"group_dispatch,omitempty"`
}

// dispatchRequest is the typed mirror of POST /api/v1/sessions/dispatch's body.
type dispatchRequest struct {
	Command string `json:"command" binding:"required"`
	Timeout int    `json:"timeout"` // seconds; defaults to 3 when ≤0
}

// dispatchResult is one row of the dispatch response.
type dispatchResult struct {
	SessionHash string `json:"session_hash"`
	Output      string `json:"output"`
	Error       string `json:"error,omitempty"`
}

// dispatchResponse wraps the list of per-session results.
type dispatchResponse struct {
	Status  bool             `json:"status"`
	Count   int              `json:"count"`
	Results []dispatchResult `json:"results"`
}

// errorResponse is the standard error body returned with non-2xx statuses.
type errorResponse struct {
	Error string `json:"error"`
}

// PatchSession updates mutable fields on an existing session.
//
// @Summary     Patch session
// @Description Update the alias and/or group_dispatch flag on a connected session.
// @Tags        sessions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id   path      string              true  "Session hash"
// @Param       body body      patchSessionRequest true  "Fields to patch (any subset)"
// @Success     200  {object}  sessionEntry "status + updated session"
// @Failure     400  {object}  errorResponse
// @Failure     404  {object}  errorResponse
// @Router      /api/v1/sessions/{id} [patch]
func PatchSession(c *gin.Context) {
	hash := c.Param("id")
	if hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id required"})
		return
	}

	var req struct {
		Alias         *string `json:"alias"`
		GroupDispatch *bool   `json:"group_dispatch"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if client := core.FindAgentClientByHash(hash); client != nil {
		if req.Alias != nil {
			client.Alias = *req.Alias
		}
		if req.GroupDispatch != nil {
			client.GroupDispatch = *req.GroupDispatch
		}
		c.JSON(http.StatusOK, gin.H{"status": true, "msg": client})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

// GatherSession re-runs the client-info probe on a session.
//
// @Summary     Gather session info
// @Description Triggers an on-demand re-probe of os/user/version/network for a session.
// @Tags        sessions
// @Produce     json
// @Security    BearerAuth
// @Param       id   path      string  true "Session hash"
// @Success     200  {object}  sessionEntry
// @Failure     404  {object}  errorResponse
// @Router      /api/v1/sessions/{id}/gather [post]
func GatherSession(c *gin.Context) {
	hash := c.Param("id")

	if client := core.FindAgentClientByHash(hash); client != nil {
		client.GatherClientInfo(client.GetHashFormat())
		c.JSON(http.StatusOK, gin.H{"status": true, "msg": client})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

// DispatchCommand broadcasts a shell command to every session whose
// group_dispatch flag is true.
//
// @Summary     Dispatch command to flagged sessions
// @Description Runs a command on every session with group_dispatch=true. Per-session timeouts surface as {error:"timeout"} inside results, not as a request error.
// @Tags        sessions
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body body      dispatchRequest  true "Command + timeout"
// @Success     200  {object}  dispatchResponse
// @Failure     400  {object}  errorResponse
// @Router      /api/v1/sessions/dispatch [post]
func DispatchCommand(c *gin.Context) {
	var req struct {
		Command string `json:"command" binding:"required"`
		Timeout int    `json:"timeout"` // seconds, default 3
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}
	if req.Timeout <= 0 {
		req.Timeout = 3
	}

	type result struct {
		SessionHash string `json:"session_hash"`
		Output      string `json:"output"`
		Error       string `json:"error,omitempty"`
	}

	var results []result
	for _, client := range core.AllAgents() {
		if client.GroupDispatch {
			ch := make(chan string, 1)
			go func() { ch <- client.System(req.Command) }()
			select {
			case output := <-ch:
				results = append(results, result{SessionHash: client.Hash, Output: output})
			case <-time.After(time.Duration(req.Timeout) * time.Second):
				results = append(results, result{SessionHash: client.Hash, Error: "timeout"})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": true, "count": len(results), "results": results})
}
