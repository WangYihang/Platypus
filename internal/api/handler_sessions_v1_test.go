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

func setupSessionsV1Router(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	core.Ctx = app.New(nil)
	auth := NewAuth()
	r := gin.New()
	RegisterV1Routes(r, auth)
	return r, auth.CreateToken()
}

// smokeReq is a terser httptest helper for v1 tests.
func smokeReq(t *testing.T, r *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	var req *http.Request
	if rdr == nil {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, rdr)
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestSessionsV1_ListEmpty(t *testing.T) {
	r, tok := setupSessionsV1Router(t)
	w := smokeReq(t, r, "GET", "/api/v1/sessions", tok, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"sessions":`) {
		t.Fatalf("response missing sessions field: %s", w.Body.String())
	}
}

func TestSessionsV1_GetNotFound(t *testing.T) {
	r, tok := setupSessionsV1Router(t)
	w := smokeReq(t, r, "GET", "/api/v1/sessions/bogus", tok, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSessionsV1_DeleteNotFound(t *testing.T) {
	r, tok := setupSessionsV1Router(t)
	w := smokeReq(t, r, "DELETE", "/api/v1/sessions/bogus", tok, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSessionsV1_ExecBadBody(t *testing.T) {
	r, tok := setupSessionsV1Router(t)
	w := smokeReq(t, r, "POST", "/api/v1/sessions/x/exec", tok, "{}")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing command, got %d", w.Code)
	}
}

func TestSessionsV1_ExecNotFound(t *testing.T) {
	r, tok := setupSessionsV1Router(t)
	w := smokeReq(t, r, "POST", "/api/v1/sessions/bogus/exec", tok, `{"command":"id"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestSessionsV1_AuthRequired asserts all new endpoints still need Bearer.
func TestSessionsV1_AuthRequired(t *testing.T) {
	r, _ := setupSessionsV1Router(t)
	cases := []struct{ method, path, body string }{
		{"GET", "/api/v1/sessions", ""},
		{"GET", "/api/v1/sessions/x", ""},
		{"DELETE", "/api/v1/sessions/x", ""},
		{"POST", "/api/v1/sessions/x/exec", `{"command":"id"}`},
	}
	for _, tc := range cases {
		w := smokeReq(t, r, tc.method, tc.path, "", tc.body)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token → %d; want 401", tc.method, tc.path, w.Code)
		}
	}
}
