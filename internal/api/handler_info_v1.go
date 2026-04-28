package api

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/pkg/version"
)

// LiveAgentCounter is set by main() so the info handler can report
// the live session count without pulling in a direct dependency on
// core.AgentLinkService. Defaults to "always zero" for tests that
// don't bother wiring it.
var LiveAgentCounter func() int = func() int { return 0 }

// Counts returns the cross-project host / session telemetry the
// status bar polls at 1 Hz. main() wires this to storage.DB.Counts;
// tests stub it. Defaults to an always-zero implementation so the
// handler doesn't crash when no DB is attached.
var Counts func(ctx context.Context) (storage.Counts, error) = func(_ context.Context) (storage.Counts, error) {
	return storage.Counts{}, nil
}

// startedAt records when the process came up. Captured at import time
// because the info endpoint has no better place to latch it and we want
// uptime to be stable across requests.
var startedAt = time.Now()

// PublicAddr is the host:port agents should dial to reach this
// server's unified-ingress port. Set once during startup by main.go
// so the /api/v1/info response and install-script templates don't
// have to re-parse cfg.Ingress.
var PublicAddr string

// serverInfoResponse is the shape of GET /api/v1/info. A roll-up of
// build metadata + runtime stats + cross-project counts so the
// desktop status bar can render a 1 Hz "is the whole server healthy"
// dashboard without fanning out to /sessions or /hosts.
type serverInfoResponse struct {
	// Build metadata.
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	GitRepo string `json:"git_repo"`

	// Process lifetime.
	StartedAt     string `json:"started_at"`
	StartedAtUnix int64  `json:"started_at_unix"`

	// Runtime stats — sampled per request.
	Goroutines    int    `json:"goroutines"`
	MemAllocBytes uint64 `json:"mem_alloc_bytes"`

	// Network identity.
	PublicAddr string `json:"public_addr"`

	// Counts.
	//
	// SessionCount stays as the legacy "live agent count" alias for
	// backwards-compat with older clients; new fields are
	// LiveSessionCount / TotalSessionCount and HostCount /
	// LiveHostCount.
	SessionCount      int `json:"session_count"`
	LiveSessionCount  int `json:"live_session_count"`
	TotalSessionCount int `json:"total_session_count"`
	HostCount         int `json:"host_count"`
	LiveHostCount     int `json:"live_host_count"`
}

// GetServerInfoV1 returns build and runtime metadata for the server.
//
// @Summary     Get server info
// @Description Returns the server's build version/commit/date, process start time, public ingress address, runtime stats (goroutines, memory), and cross-project host/session counts. Intended for 1 Hz status-bar polling.
// @Tags        info
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} serverInfoResponse
// @Router      /api/v1/info [get]
func GetServerInfoV1(c *gin.Context) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	live := LiveAgentCounter()

	// Counts() is a single round-trip to the DB so it's cheap to
	// call per request. If it fails we still return the rest of the
	// response — the status bar degrades gracefully and the operator
	// gets at least the runtime stats.
	counts, _ := Counts(c.Request.Context())

	c.JSON(http.StatusOK, serverInfoResponse{
		Version: version.Version,
		Commit:  version.Commit,
		Date:    version.Date,
		GitRepo: version.Repo,

		StartedAt:     startedAt.UTC().Format(time.RFC3339),
		StartedAtUnix: startedAt.Unix(),

		Goroutines:    runtime.NumGoroutine(),
		MemAllocBytes: ms.Alloc,

		PublicAddr: PublicAddr,

		// Legacy session_count remains tied to the in-memory live
		// agent registry (LiveAgentCounter) for backwards-compat
		// with older clients. New live_session_count comes from
		// counts.LiveSessions (DB ground truth — sessions with no
		// disconnected_at). The two normally agree; the DB value
		// is authoritative.
		SessionCount:      live,
		LiveSessionCount:  counts.LiveSessions,
		TotalSessionCount: counts.Sessions,
		HostCount:         counts.Hosts,
		LiveHostCount:     counts.LiveHosts,
	})
}

// versionResponse is the public, auth-free shape of GET
// /api/v1/version. Strict subset of serverInfoResponse: no uptime, no
// public_addr, no session count. Lets clients (release banner, CLI
// upgrade nags, dashboards) check the running build without trading
// for a token first.
type versionResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// GetVersionV1 returns the build trio. Unlike /api/v1/info this is
// public — it leaks no environment-specific data, just what binary is
// running.
//
// @Summary     Get server version
// @Description Returns the server's build version, commit, and date. Public — no auth required. Strict subset of /api/v1/info; safe to expose to unauthenticated callers.
// @Tags        info
// @Produce     json
// @Success     200 {object} versionResponse
// @Router      /api/v1/version [get]
func GetVersionV1(c *gin.Context) {
	c.JSON(http.StatusOK, versionResponse{
		Version: version.Version,
		Commit:  version.Commit,
		Date:    version.Date,
	})
}
