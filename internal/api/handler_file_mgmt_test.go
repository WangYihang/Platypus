package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
)

// setupFileMgmtRouter mirrors setupFileRouter in handler_file_test.go but
// wires only the new directory / delete / rename / mkdir / chmod routes.
// Same caveat: the agent-backed success path requires a live AgentClient,
// so we exercise the 400/404 boundaries that don't need one.
func setupFileMgmtRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	core.Ctx = app.New(nil)
	auth := NewAuth()
	r := gin.New()
	g := r.Group("/api/v1")
	g.Use(auth.Middleware())
	g.GET("/sessions/:id/files/list", ListDirHandler)
	g.GET("/sessions/:id/files/stat", StatHandler)
	g.DELETE("/sessions/:id/files", DeleteFileHandler)
	g.POST("/sessions/:id/files/rename", RenameFileHandler)
	g.POST("/sessions/:id/files/mkdir", MkdirHandler)
	g.POST("/sessions/:id/files/chmod", ChmodHandler)
	return r, auth.CreateToken()
}

func mgmtReq(t *testing.T, r *gin.Engine, method, path, token string, body []byte, ct string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var req *http.Request
	if body == nil {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, bytes.NewReader(body))
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w
}

func TestListDir_MissingPath(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "GET", "/api/v1/sessions/x/files/list", tok, nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListDir_UnknownSession(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "GET", "/api/v1/sessions/bogus/files/list?path=/tmp", tok, nil, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestStat_MissingPath(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "GET", "/api/v1/sessions/x/files/stat", tok, nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDelete_MissingPath(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "DELETE", "/api/v1/sessions/x/files", tok, nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDelete_UnknownSession(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "DELETE", "/api/v1/sessions/bogus/files?path=/tmp/x", tok, nil, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRename_BadBody(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	// Missing required "from"/"to"
	w := mgmtReq(t, r, "POST", "/api/v1/sessions/x/files/rename", tok, []byte(`{}`), "application/json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing fields, got %d", w.Code)
	}
}

func TestRename_UnknownSession(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "POST", "/api/v1/sessions/bogus/files/rename", tok,
		[]byte(`{"from":"/a","to":"/b"}`), "application/json")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMkdir_MissingPath(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "POST", "/api/v1/sessions/x/files/mkdir", tok, nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMkdir_InvalidMode(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	// "999" is not an octal digit string.
	w := mgmtReq(t, r, "POST", "/api/v1/sessions/x/files/mkdir?path=/tmp/x&mode=999", tok, nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad octal, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChmod_MissingMode(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "POST", "/api/v1/sessions/x/files/chmod?path=/tmp/x", tok, nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestChmod_BadMode(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "POST", "/api/v1/sessions/x/files/chmod?path=/tmp/x&mode=abc", tok, nil, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad octal, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChmod_UnknownSession(t *testing.T) {
	r, tok := setupFileMgmtRouter(t)
	w := mgmtReq(t, r, "POST", "/api/v1/sessions/bogus/files/chmod?path=/tmp/x&mode=644", tok, nil, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
