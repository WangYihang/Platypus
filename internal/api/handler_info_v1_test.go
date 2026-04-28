package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/storage"
)

// infoSmokeReq runs a GET with an optional Bearer token.
func infoSmokeReq(_ *testing.T, r *gin.Engine, method, path, tok string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func setupInfoV1Router(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	core.Ctx = app.New(nil)
	auth := NewAuth()
	r := gin.New()
	RegisterV1Routes(r, auth)
	return r, auth.CreateToken()
}

func TestInfoV1_AuthRequired(t *testing.T) {
	r, _ := setupInfoV1Router(t)
	w := infoSmokeReq(t, r, "GET", "/api/v1/info", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}
}

func TestInfoV1_Happy(t *testing.T) {
	r, tok := setupInfoV1Router(t)
	w := infoSmokeReq(t, r, "GET", "/api/v1/info", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body serverInfoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body=%s", err, w.Body.String())
	}
	// Version/Commit/Date come from pkg/version; we just assert the
	// fields are populated so the endpoint can't silently drop a field
	// in a refactor.
	if body.Version == "" {
		t.Errorf("version missing")
	}
	if body.Commit == "" {
		t.Errorf("commit missing")
	}
	if body.Date == "" {
		t.Errorf("date missing")
	}
	if body.StartedAt == "" {
		t.Errorf("started_at missing")
	}
	// Fresh core.Ctx has no servers or agents.
	if body.SessionCount != 0 {
		t.Errorf("session_count=%d, want 0", body.SessionCount)
	}
}

// TestInfoV1_RuntimeAndCounts pins the new status-bar telemetry
// fields:
//
//   1. Runtime stats — goroutines (>0), mem_alloc_bytes (>0), and a
//      stable started_at_unix that doesn't drift between calls.
//   2. Cross-project counts — host_count / live_host_count /
//      total_session_count / live_session_count, threaded through
//      the Counts func var so production main.go can wire it to
//      db.Counts() without dragging the storage layer into this
//      handler.
//   3. The repo path so the frontend can render clickable
//      release links.
func TestInfoV1_RuntimeAndCounts(t *testing.T) {
	r, tok := setupInfoV1Router(t)
	prev := Counts
	defer func() { Counts = prev }()
	Counts = func(_ context.Context) (storage.Counts, error) {
		return storage.Counts{
			Hosts: 12, LiveHosts: 9,
			Sessions: 47, LiveSessions: 3,
		}, nil
	}

	w := infoSmokeReq(t, r, "GET", "/api/v1/info", tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, w.Body.String())
	}

	if g, _ := got["goroutines"].(float64); g <= 0 {
		t.Errorf("goroutines = %v, want > 0", got["goroutines"])
	}
	if m, _ := got["mem_alloc_bytes"].(float64); m <= 0 {
		t.Errorf("mem_alloc_bytes = %v, want > 0", got["mem_alloc_bytes"])
	}
	if s, _ := got["started_at_unix"].(float64); s <= 0 {
		t.Errorf("started_at_unix = %v, want > 0", got["started_at_unix"])
	}
	if got["host_count"].(float64) != 12 {
		t.Errorf("host_count = %v, want 12", got["host_count"])
	}
	if got["live_host_count"].(float64) != 9 {
		t.Errorf("live_host_count = %v, want 9", got["live_host_count"])
	}
	if got["total_session_count"].(float64) != 47 {
		t.Errorf("total_session_count = %v, want 47", got["total_session_count"])
	}
	if got["live_session_count"].(float64) != 3 {
		t.Errorf("live_session_count = %v, want 3", got["live_session_count"])
	}
	if repo, _ := got["git_repo"].(string); !strings.Contains(repo, "/") {
		t.Errorf("git_repo = %q, want owner/repo form", got["git_repo"])
	}
}

// TestInfoV1_StableStartedAt — uptime must be stable across calls.
func TestInfoV1_StableStartedAt(t *testing.T) {
	r, tok := setupInfoV1Router(t)
	w1 := infoSmokeReq(t, r, "GET", "/api/v1/info", tok)
	time.Sleep(10 * time.Millisecond)
	w2 := infoSmokeReq(t, r, "GET", "/api/v1/info", tok)

	var a, b map[string]any
	_ = json.Unmarshal(w1.Body.Bytes(), &a)
	_ = json.Unmarshal(w2.Body.Bytes(), &b)
	if a["started_at_unix"] != b["started_at_unix"] {
		t.Errorf("started_at_unix drifted: %v → %v",
			a["started_at_unix"], b["started_at_unix"])
	}
}
