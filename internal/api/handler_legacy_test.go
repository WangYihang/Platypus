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

// setupLegacyRouter wires the legacy routes on a fresh empty app context so
// that tests can hit the not-found / bad-request paths without needing a live
// reverse-shell listener. Returns the engine and a valid bearer token.
func setupLegacyRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	core.Ctx = app.New(nil)
	auth := NewAuth()
	r := gin.New()
	r.POST("/api/v1/auth/token", auth.TokenEndpoint())
	RegisterLegacyRoutes(r, auth)
	return r, auth.CreateToken()
}

func doRequest(t *testing.T, r *gin.Engine, method, path, token, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, strings.NewReader(body))
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestLegacyGetServerNotFound(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	w := doRequest(t, r, "GET", "/api/server/bogushash", tok, "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/server/:hash unknown → expected 404, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestLegacyDeleteServerNotFound(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	w := doRequest(t, r, "DELETE", "/api/server/bogushash", tok, "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("DELETE /api/server/:hash unknown → expected 404, got %d", w.Code)
	}
}

func TestLegacyGetServerClientsNotFound(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	w := doRequest(t, r, "GET", "/api/server/bogushash/client", tok, "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/server/:hash/client unknown → expected 404, got %d", w.Code)
	}
}

func TestLegacyCreateServerMissingForm(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	w := doRequest(t, r, "POST", "/api/server", tok, "application/x-www-form-urlencoded", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/server missing form → expected 400, got %d", w.Code)
	}
}

func TestLegacyCreateServerInvalidPort(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	body := "host=0.0.0.0&port=99999&encrypted=false"
	w := doRequest(t, r, "POST", "/api/server", tok, "application/x-www-form-urlencoded", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/server bad port → expected 400, got %d", w.Code)
	}
}

func TestLegacyGetClientNotFound(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	w := doRequest(t, r, "GET", "/api/client/bogushash", tok, "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/client/:hash unknown → expected 404, got %d", w.Code)
	}
}

func TestLegacyDeleteClientNotFound(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	w := doRequest(t, r, "DELETE", "/api/client/bogushash", tok, "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("DELETE /api/client/:hash unknown → expected 404, got %d", w.Code)
	}
}

func TestLegacyExecClientMissingCmd(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	w := doRequest(t, r, "POST", "/api/client/bogushash", tok, "application/x-www-form-urlencoded", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/client/:hash missing cmd → expected 400, got %d", w.Code)
	}
}

func TestLegacyExecClientNotFound(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	body := "cmd=whoami"
	w := doRequest(t, r, "POST", "/api/client/bogushash", tok, "application/x-www-form-urlencoded", body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("POST /api/client/:hash unknown → expected 404, got %d", w.Code)
	}
}

func TestLegacyUpgradeClientNotFound(t *testing.T) {
	r, tok := setupLegacyRouter(t)
	w := doRequest(t, r, "GET", "/api/client/bogushash/upgrade/targethash", tok, "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/client/:hash/upgrade/:target unknown → expected 404, got %d", w.Code)
	}
}

// TestLegacyAuthRequired ensures the bearer middleware still guards every legacy route.
func TestLegacyAuthRequired(t *testing.T) {
	r, _ := setupLegacyRouter(t)
	cases := []struct {
		method string
		path   string
	}{
		{"GET", "/api/server"},
		{"GET", "/api/server/x"},
		{"POST", "/api/server"},
		{"DELETE", "/api/server/x"},
		{"GET", "/api/server/x/client"},
		{"GET", "/api/client"},
		{"GET", "/api/client/x"},
		{"POST", "/api/client/x"},
		{"DELETE", "/api/client/x"},
		{"GET", "/api/client/x/upgrade/y"},
	}
	for _, tc := range cases {
		w := doRequest(t, r, tc.method, tc.path, "", "", "")
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token → expected 401, got %d", tc.method, tc.path, w.Code)
		}
	}
}
