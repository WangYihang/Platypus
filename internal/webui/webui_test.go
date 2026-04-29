package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestIndexHTMLIsEmbedded(t *testing.T) {
	info, err := fs.Stat(distFS, "dist/index.html")
	if err != nil {
		t.Fatalf("dist/index.html not embedded: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("dist/index.html is empty")
	}
}

func newTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	e := gin.New()
	// Mimic main.go: register an API route first, then the webui
	// fallback. Verifies precedence with the explicit dummy /api/v1/echo.
	e.GET("/api/v1/echo", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	RegisterRoutes(e)
	return e
}

func TestRegisterRoutes_RootServesHTML(t *testing.T) {
	e := newTestEngine(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	e.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /: status %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET /: Content-Type %q, want text/html...", ct)
	}
	if !strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("GET /: body missing <html, got %q", w.Body.String())
	}
}

func TestRegisterRoutes_DeepLinkFallsBackToIndex(t *testing.T) {
	e := newTestEngine(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects/foo/hosts", nil)
	e.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("deep link: status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("deep link: body missing <html, got %q", w.Body.String())
	}
}

func TestRegisterRoutes_APIPrefixReturnsJSON404(t *testing.T) {
	e := newTestEngine(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	e.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("api miss: status %d, want 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("api miss: Content-Type %q, want application/json", ct)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("api miss leaked HTML body: %q", w.Body.String())
	}
}

func TestRegisterRoutes_APIPrefixDoesNotShadowRealRoute(t *testing.T) {
	e := newTestEngine(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/echo", nil)
	e.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("real api: status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("real api: body %q missing ok:true", w.Body.String())
	}
}

func TestRegisterRoutes_SwaggerPrefixReturnsJSON404(t *testing.T) {
	e := newTestEngine(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/swagger/missing.json", nil)
	e.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("swagger miss: status %d, want 404", w.Code)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("swagger miss leaked HTML body: %q", w.Body.String())
	}
}
