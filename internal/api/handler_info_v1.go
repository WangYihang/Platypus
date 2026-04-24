package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/pkg/version"
)

// startedAt records when the process came up. Captured at import time
// because the info endpoint has no better place to latch it and we want
// uptime to be stable across requests.
var startedAt = time.Now()

// serverInfoResponse is the shape of GET /api/v1/info. A thin roll-up
// of build metadata + live counts so the desktop status bar can render
// local/remote health without fanning out to /sessions.
type serverInfoResponse struct {
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	Date         string `json:"date"`
	StartedAt    string `json:"started_at"`
	SessionCount int    `json:"session_count"`
}

// GetServerInfoV1 returns build and runtime metadata for the server.
//
// @Summary     Get server info
// @Description Returns the server's build version/commit/date, process start time, and current session count. Intended for lightweight dashboard/status-bar polling.
// @Tags        info
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} serverInfoResponse
// @Router      /api/v1/info [get]
func GetServerInfoV1(c *gin.Context) {
	sessions := 0
	for _, srv := range core.GetServers() {
		sessions += len(srv.AgentClients)
	}
	c.JSON(http.StatusOK, serverInfoResponse{
		Version:      version.Version,
		Commit:       version.Commit,
		Date:         version.Date,
		StartedAt:    startedAt.UTC().Format(time.RFC3339),
		SessionCount: sessions,
	})
}
