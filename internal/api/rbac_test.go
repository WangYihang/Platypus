package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/user"
)

func rbacTestSetup(t *testing.T) (*RBAC, *TokenIssuer) {
	t.Helper()
	issuer, err := NewTokenIssuer("a", "b", 5*time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	return NewRBAC(issuer), issuer
}

// mountProtected mounts a single /probe route guarded by the given middleware
// chain. The handler writes the authenticated user's role into the body so
// tests can assert both status and identity.
func mountProtected(mw ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	chain := append(mw, func(c *gin.Context) {
		claims, ok := ClaimsFromContext(c)
		if !ok {
			c.String(http.StatusInternalServerError, "no claims")
			return
		}
		c.String(http.StatusOK, string(claims.Role))
	})
	r.GET("/probe", chain...)
	return r
}

func probeReq(r http.Handler, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/probe", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	rb, _ := rbacTestSetup(t)
	r := mountProtected(rb.RequireAuth())

	w := probeReq(r, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRequireAuth_MalformedHeader(t *testing.T) {
	rb, _ := rbacTestSetup(t)
	r := mountProtected(rb.RequireAuth())

	req := httptest.NewRequest("GET", "/probe", nil)
	req.Header.Set("Authorization", "Basic user:pass")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	rb, _ := rbacTestSetup(t)
	r := mountProtected(rb.RequireAuth())

	w := probeReq(r, "not-a-jwt")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestRequireAuth_ValidTokenPutsClaimsOnContext(t *testing.T) {
	rb, issuer := rbacTestSetup(t)
	r := mountProtected(rb.RequireAuth())

	tok, err := issuer.IssueAccess(AccessClaims{
		UserID: "u1", Username: "alice", Role: user.RoleOperator,
	})
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	w := probeReq(r, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != string(user.RoleOperator) {
		t.Fatalf("body=%q; want %q", w.Body.String(), user.RoleOperator)
	}
}

func TestRequireGlobalRole_BlocksLowerRole(t *testing.T) {
	rb, issuer := rbacTestSetup(t)
	r := mountProtected(rb.RequireAuth(), rb.RequireGlobalRole(user.RoleAdmin))

	tok, _ := issuer.IssueAccess(AccessClaims{
		UserID: "u1", Role: user.RoleViewer,
	})
	w := probeReq(r, tok)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer hitting admin-only route status=%d; want 403", w.Code)
	}
}

func TestRequireGlobalRole_AllowsMatch(t *testing.T) {
	rb, issuer := rbacTestSetup(t)
	r := mountProtected(rb.RequireAuth(), rb.RequireGlobalRole(user.RoleAdmin))

	tok, _ := issuer.IssueAccess(AccessClaims{
		UserID: "u1", Role: user.RoleAdmin,
	})
	w := probeReq(r, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("admin hitting admin-only route status=%d; want 200", w.Code)
	}
}

// Role ordering: admin > operator > viewer. RequireGlobalRole should allow
// any role at or above the threshold.
func TestRequireGlobalRole_AllowsHigher(t *testing.T) {
	rb, issuer := rbacTestSetup(t)
	r := mountProtected(rb.RequireAuth(), rb.RequireGlobalRole(user.RoleOperator))

	tok, _ := issuer.IssueAccess(AccessClaims{
		UserID: "u1", Role: user.RoleAdmin,
	})
	w := probeReq(r, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("admin hitting operator-only route status=%d; want 200", w.Code)
	}
}
