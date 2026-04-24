package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/pkg/version"
)

// LiveAgentCounter is set by main() so the info handler can report
// the live session count without pulling in a direct dependency on
// core.AgentLinkService. Defaults to "always zero" for tests that
// don't bother wiring it.
var LiveAgentCounter func() int = func() int { return 0 }

// startedAt records when the process came up. Captured at import time
// because the info endpoint has no better place to latch it and we want
// uptime to be stable across requests.
var startedAt = time.Now()

// PublicAddr is the host:port agents should dial to reach this
// server's unified-ingress port. Set once during startup by main.go
// so the /api/v1/info response and install-script templates don't
// have to re-parse cfg.Ingress.
var PublicAddr string

// serverInfoResponse is the shape of GET /api/v1/info. A thin roll-up
// of build metadata + live counts so the desktop status bar can render
// local/remote health without fanning out to /sessions.
type serverInfoResponse struct {
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	Date         string `json:"date"`
	StartedAt    string `json:"started_at"`
	PublicAddr   string `json:"public_addr"`
	SessionCount int    `json:"session_count"`
}

// GetServerInfoV1 returns build and runtime metadata for the server.
//
// @Summary     Get server info
// @Description Returns the server's build version/commit/date, process start time, public ingress address, and current session count. Intended for lightweight dashboard/status-bar polling.
// @Tags        info
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} serverInfoResponse
// @Router      /api/v1/info [get]
func GetServerInfoV1(c *gin.Context) {
	c.JSON(http.StatusOK, serverInfoResponse{
		Version:      version.Version,
		Commit:       version.Commit,
		Date:         version.Date,
		StartedAt:    startedAt.UTC().Format(time.RFC3339),
		PublicAddr:   PublicAddr,
		SessionCount: LiveAgentCounter(),
	})
}
