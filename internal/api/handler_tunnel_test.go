package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
)

// setupTunnelRouter wires /api/v1/sessions/:id/tunnels on a fresh empty app
// context so tests can populate tunnel maps before hitting the handler.
func setupTunnelRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	core.Ctx = app.New(nil)
	auth := NewAuth()
	r := gin.New()
	g := r.Group("/api/v1")
	g.Use(auth.Middleware())
	g.GET("/sessions/:id/tunnels", ListTunnels)
	return r, auth.CreateToken()
}

// TestListTunnelsScopedToSession asserts that GET /sessions/:id/tunnels
// returns only tunnels owned by the named session, not every tunnel in the
// global context. Pre-fix, the handler iterated the maps without filtering.
func TestListTunnelsScopedToSession(t *testing.T) {
	r, tok := setupTunnelRouter(t)

	// Seed two sessions worth of tunnels.
	core.Ctx.PullTunnelConfig["127.0.0.1:1001"] = app.PullTunnelConfig{
		Agent:   &core.AgentClient{Hash: "sessionA"},
		Address: "remote-a:80",
	}
	core.Ctx.PullTunnelConfig["127.0.0.1:1002"] = app.PullTunnelConfig{
		Agent:   &core.AgentClient{Hash: "sessionB"},
		Address: "remote-b:80",
	}
	core.Ctx.PushTunnelConfig["remote-a:22"] = app.PushTunnelConfig{
		Agent:   &core.AgentClient{Hash: "sessionA"},
		Address: "127.0.0.1:2001",
	}
	core.Ctx.PushTunnelConfig["remote-b:22"] = app.PushTunnelConfig{
		Agent:   &core.AgentClient{Hash: "sessionB"},
		Address: "127.0.0.1:2002",
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/sessions/sessionA/tunnels", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "127.0.0.1:1001") {
		t.Errorf("sessionA pull tunnel missing: %s", body)
	}
	if !strings.Contains(body, "127.0.0.1:2001") {
		t.Errorf("sessionA push tunnel missing: %s", body)
	}
	if strings.Contains(body, "127.0.0.1:1002") {
		t.Errorf("sessionB pull tunnel leaked into sessionA view: %s", body)
	}
	if strings.Contains(body, "127.0.0.1:2002") {
		t.Errorf("sessionB push tunnel leaked into sessionA view: %s", body)
	}
}

// TestListTunnelsEmptyForUnknownSession asserts no tunnels are returned when
// the session hash doesn't match any owner.
func TestListTunnelsEmptyForUnknownSession(t *testing.T) {
	r, tok := setupTunnelRouter(t)
	core.Ctx.PullTunnelConfig["127.0.0.1:1001"] = app.PullTunnelConfig{
		Agent:   &core.AgentClient{Hash: "sessionA"},
		Address: "remote:80",
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/sessions/nobody/tunnels", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "127.0.0.1:1001") {
		t.Errorf("unknown session received sessionA's tunnel: %s", w.Body.String())
	}
}
