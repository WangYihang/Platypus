package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
)

// setupFileRouter wires the v1 file routes on a fresh empty app context.
// All tests here exercise paths that don't require a live agent connection
// (missing query params, unknown session). The agent-error → 502 path is
// covered by manual review because mocking the underlying net.Conn for a
// AgentClient would require interface refactoring out of scope here.
func setupFileRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	core.Ctx = app.New(nil)
	auth := NewAuth()
	r := gin.New()
	g := r.Group("/api/v1")
	g.Use(auth.Middleware())
	g.GET("/sessions/:id/files/size", GetFileSize)
	g.GET("/sessions/:id/files", ReadFile)
	g.POST("/sessions/:id/files", WriteFile)
	return r, auth.CreateToken()
}

func fileReq(t *testing.T, r *gin.Engine, method, path, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var req *http.Request
	if body == nil {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w
}

func TestFileSize_MissingPath(t *testing.T) {
	r, tok := setupFileRouter(t)
	w := fileReq(t, r, "GET", "/api/v1/sessions/x/files/size", tok, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestFileSize_UnknownSession(t *testing.T) {
	r, tok := setupFileRouter(t)
	w := fileReq(t, r, "GET", "/api/v1/sessions/bogus/files/size?path=/etc/hosts", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "error") {
		t.Errorf("error envelope missing %q field: %s", "error", body)
	}
	if strings.Contains(body, `"status":true`) {
		t.Errorf("404 should not advertise status:true: %s", body)
	}
}

func TestReadFile_MissingPath(t *testing.T) {
	r, tok := setupFileRouter(t)
	w := fileReq(t, r, "GET", "/api/v1/sessions/x/files", tok, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestReadFile_UnknownSession(t *testing.T) {
	r, tok := setupFileRouter(t)
	w := fileReq(t, r, "GET", "/api/v1/sessions/bogus/files?path=/etc/hosts", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestWriteFile_MissingPath(t *testing.T) {
	r, tok := setupFileRouter(t)
	w := fileReq(t, r, "POST", "/api/v1/sessions/x/files", tok, []byte("hi"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWriteFile_UnknownSession(t *testing.T) {
	r, tok := setupFileRouter(t)
	w := fileReq(t, r, "POST", "/api/v1/sessions/bogus/files?path=/tmp/x", tok, []byte("hi"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestFileEndpointsAuthRequired smoke-tests the auth middleware on every
// file route.
func TestFileEndpointsAuthRequired(t *testing.T) {
	r, _ := setupFileRouter(t)
	cases := []struct{ method, path string }{
		{"GET", "/api/v1/sessions/x/files/size?path=/x"},
		{"GET", "/api/v1/sessions/x/files?path=/x"},
		{"POST", "/api/v1/sessions/x/files?path=/x"},
	}
	for _, tc := range cases {
		w := fileReq(t, r, tc.method, tc.path, "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token → expected 401, got %d", tc.method, tc.path, w.Code)
		}
	}
}
