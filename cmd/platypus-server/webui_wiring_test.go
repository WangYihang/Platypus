package main_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/webui"
)

// TestWebUIWiring smoke-checks that the webui package's contract used
// by main.go (`webui.RegisterRoutes(*gin.Engine)` + a working SPA
// fallback) still holds.  The full buildRESTEngine wiring needs a DB,
// PKI, and settings registry; this test stays in the cmd/ package to
// catch import-path drift but skips the heavy deps — the api-level
// integration test in internal/api/startup_integration_test.go
// continues to cover the rest of the pipeline.
func TestWebUIWiring(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// Mirror main.go: at least one explicit API route registered
	// BEFORE the webui catch-all, so we can prove first-match order.
	engine.GET("/api/v1/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"version": "test"})
	})

	webui.RegisterRoutes(engine)

	// Root serves the embedded HTML (stub or real frontend — either way HTML).
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /: status %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET /: Content-Type %q, want text/html", ct)
	}

	// Explicit API route still wins over the SPA fallback.
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/version", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/version: status %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"version":"test"`) {
		t.Fatalf("GET /api/v1/version: body %q lost the API handler", w.Body.String())
	}

	// Misses under /api/ return JSON, not HTML.
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("api miss: status %d, want 404", w.Code)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("api miss leaked HTML: %q", w.Body.String())
	}
}
