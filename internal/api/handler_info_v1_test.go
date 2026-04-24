package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
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
