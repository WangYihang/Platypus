package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCORSDoesNotAllowCredentials verifies that the CORS response does not
// advertise Access-Control-Allow-Credentials. The API authenticates via Bearer
// tokens (not cookies), and wildcard AllowOrigin + credentials is a browser
// anti-pattern (browsers silently drop the credentials).
func TestCORSDoesNotAllowCredentials(t *testing.T) {
	engine := CreateRESTfulAPIServer()

	req, _ := http.NewRequest("OPTIONS", "/api/v1/auth/token", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Access-Control-Allow-Credentials should be absent, got %q", got)
	}
}

// TestCORSAllowsWildcardOrigin verifies the permissive wildcard origin is kept.
func TestCORSAllowsWildcardOrigin(t *testing.T) {
	engine := CreateRESTfulAPIServer()

	req, _ := http.NewRequest("OPTIONS", "/api/v1/auth/token", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin: expected %q, got %q", "*", got)
	}
}
