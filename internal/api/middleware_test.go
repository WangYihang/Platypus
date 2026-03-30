package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupRouter(auth *Auth) *gin.Engine {
	r := gin.New()
	r.POST("/api/v1/auth/token", auth.TokenEndpoint())

	protected := r.Group("/api/v1")
	protected.Use(auth.Middleware())
	{
		protected.GET("/test", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})
	}
	return r
}

func TestAuthTokenEndpoint(t *testing.T) {
	auth := NewAuth()
	r := setupRouter(auth)

	// Valid secret → token
	body := `{"secret":"` + auth.GetSecret() + `"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("expected token in response: %s", w.Body.String())
	}
}

func TestAuthTokenEndpointInvalidSecret(t *testing.T) {
	auth := NewAuth()
	r := setupRouter(auth)

	body := `{"secret":"wrong-secret"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	auth := NewAuth()
	r := setupRouter(auth)
	token := auth.CreateToken()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddlewareMissingToken(t *testing.T) {
	auth := NewAuth()
	r := setupRouter(auth)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	auth := NewAuth()
	r := setupRouter(auth)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-here")
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareBadFormat(t *testing.T) {
	auth := NewAuth()
	r := setupRouter(auth)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Basic auth instead of Bearer
	r.ServeHTTP(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMultipleTokens(t *testing.T) {
	auth := NewAuth()
	r := setupRouter(auth)

	token1 := auth.CreateToken()
	token2 := auth.CreateToken()

	// Both tokens should work
	for _, token := range []string{token1, token2} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Fatalf("token %s: expected 200, got %d", token[:8], w.Code)
		}
	}
}

func TestAuthTokenEndpointEmptyBody(t *testing.T) {
	auth := NewAuth()
	r := setupRouter(auth)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/token", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
