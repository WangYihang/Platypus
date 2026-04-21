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

// TestLegacyDeprecationHeaders asserts every legacy endpoint carries a
// Deprecation: true header plus a Link to its v1 successor. Monitoring
// and clients can opt into a quiet migration without parsing response
// bodies.
func TestLegacyDeprecationHeaders(t *testing.T) {
	core.Ctx = app.New(nil)
	auth := NewAuth()
	r := gin.New()
	RegisterLegacyRoutes(r, auth)
	tok := auth.CreateToken()

	// Every legacy endpoint should mint the Deprecation header before its
	// handler runs. We avoid GET /api/server (the one success path that
	// hits the uninitialised Distributor in test mode) and instead exercise
	// endpoints whose error / not-found / no-body responses still pass
	// through the deprecate() middleware.
	cases := []struct {
		method    string
		path      string
		successor string
	}{
		{"GET", "/api/client", "/api/v1/sessions"},
		{"GET", "/api/client/x", "/api/v1/sessions/{id}"},
		{"DELETE", "/api/client/x", "/api/v1/sessions/{id}"},
		{"POST", "/api/server", "/api/v1/listeners"},
		{"GET", "/api/server/x", "/api/v1/listeners/{id}"},
		{"DELETE", "/api/server/x", "/api/v1/listeners/{id}"},
		{"GET", "/api/server/x/client", "/api/v1/listeners/{id}/sessions"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		r.ServeHTTP(w, req)
		if got := w.Header().Get("Deprecation"); got != "true" {
			t.Errorf("%s %s: Deprecation header = %q; want true", tc.method, tc.path, got)
		}
		link := w.Header().Get("Link")
		if !strings.Contains(link, tc.successor) {
			t.Errorf("%s %s: Link = %q; want substring %q", tc.method, tc.path, link, tc.successor)
		}
		if !strings.Contains(link, `rel="successor-version"`) {
			t.Errorf("%s %s: Link missing rel=\"successor-version\": %q", tc.method, tc.path, link)
		}
	}
}
