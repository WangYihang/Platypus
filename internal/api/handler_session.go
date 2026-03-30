package api

import (
	"net/http"
	"time"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/gin-gonic/gin"
)

// PatchSession handles PATCH /api/v1/sessions/:id
// Allows updating alias and group_dispatch fields.
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

	// Try TCPClient
	if client := core.FindTCPClientByHash(hash); client != nil {
		if req.Alias != nil {
			client.Alias = *req.Alias
		}
		if req.GroupDispatch != nil {
			client.GroupDispatch = *req.GroupDispatch
		}
		c.JSON(http.StatusOK, gin.H{"status": true, "msg": client})
		return
	}

	// Try TermiteClient
	if client := core.FindTermiteClientByHash(hash); client != nil {
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

// GatherSession handles POST /api/v1/sessions/:id/gather
// Triggers client info gathering on the specified session.
func GatherSession(c *gin.Context) {
	hash := c.Param("id")

	if client := core.FindTCPClientByHash(hash); client != nil {
		client.GatherClientInfo(client.GetHashFormat())
		c.JSON(http.StatusOK, gin.H{"status": true, "msg": client})
		return
	}

	if client := core.FindTermiteClientByHash(hash); client != nil {
		client.GatherClientInfo(client.GetHashFormat())
		c.JSON(http.StatusOK, gin.H{"status": true, "msg": client})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
}

// DispatchCommand handles POST /api/v1/sessions/dispatch
// Executes a command on all sessions with GroupDispatch enabled.
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
	for _, s := range core.GetServers() {
		for _, client := range s.GetAllTCPClients() {
			if client.GroupDispatch {
				output := client.SystemToken(req.Command)
				results = append(results, result{SessionHash: client.Hash, Output: output})
			}
		}
		for _, client := range s.GetAllTermiteClients() {
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
	}

	c.JSON(http.StatusOK, gin.H{"status": true, "count": len(results), "results": results})
}
