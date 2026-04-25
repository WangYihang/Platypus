package api_test

import (
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/user"
)

func TestPrincipalFromClaims(t *testing.T) {
	t.Parallel()
	claims := api.AccessClaims{
		UserID:   "u-alice",
		Username: "alice",
		Role:     user.RoleOperator,
	}
	p := api.PrincipalFromClaims(claims)
	if p.Kind != api.PrincipalUser {
		t.Errorf("Kind = %v, want PrincipalUser", p.Kind)
	}
	if p.UserID != "u-alice" || p.Username != "alice" || p.Role != user.RoleOperator {
		t.Errorf("identity mismatch: %+v", p)
	}
	// Human users derive scopes from role; the api never reads
	// scopes for them off the wire.
	if !optoken.HasScope(p.Scopes, optoken.ScopeHostsRead) {
		t.Errorf("operator scopes missing hosts:read: %v", p.Scopes)
	}
	if p.ProjectID != "" || p.TokenID != "" {
		t.Errorf("user-kind should leave ProjectID/TokenID empty: %+v", p)
	}
}

func TestPrincipalFromVerified(t *testing.T) {
	t.Parallel()
	v := &optoken.Verified{
		TokenID:   "aat_x",
		Kind:      optoken.KindAAT,
		UserID:    "u-creator",
		Role:      user.RoleViewer,
		Scopes:    []string{optoken.ScopeHostsRead, optoken.ScopeFilesRead},
		ProjectID: "p1",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	p := api.PrincipalFromVerified(v)
	if p.Kind != api.PrincipalAATKind {
		t.Errorf("Kind = %v, want PrincipalAATKind", p.Kind)
	}
	if p.TokenID != "aat_x" {
		t.Errorf("TokenID = %q", p.TokenID)
	}
	if p.UserID != "u-creator" {
		t.Errorf("UserID = %q (the AAT issuer)", p.UserID)
	}
	if p.Role != user.RoleViewer {
		t.Errorf("Role = %q", p.Role)
	}
	if p.ProjectID != "p1" {
		t.Errorf("ProjectID = %q", p.ProjectID)
	}
	if !slices.Equal(p.Scopes, v.Scopes) {
		t.Errorf("scopes mismatch: got %v want %v", p.Scopes, v.Scopes)
	}
	// Username is left empty for AAT — there's no user the request
	// is "as", just the issuer.
	if p.Username != "" {
		t.Errorf("Username = %q, want empty for AAT", p.Username)
	}
}

func TestPrincipalFromContext_Empty(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	captured := make(chan bool, 1)
	r.GET("/", func(c *gin.Context) {
		_, ok := api.PrincipalFromContext(c)
		captured <- ok
		c.Status(204)
	})
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := <-captured; got {
		t.Error("PrincipalFromContext on empty ctx = ok, want !ok")
	}
}

func TestPrincipalFromContext_RoundTrip(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	want := &api.Principal{
		Kind:    api.PrincipalAATKind,
		UserID:  "u",
		TokenID: "aat_round",
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		api.SetPrincipal(c, want)
		c.Next()
	})
	captured := make(chan *api.Principal, 1)
	r.GET("/", func(c *gin.Context) {
		got, ok := api.PrincipalFromContext(c)
		if !ok {
			t.Error("PrincipalFromContext after Set = !ok")
		}
		captured <- got
		c.Status(204)
	})
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	got := <-captured
	if got.TokenID != want.TokenID || got.Kind != want.Kind {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, want)
	}
}

func TestPrincipal_AdminAATBypassRejected(t *testing.T) {
	t.Parallel()
	// The plan disables admin-bypass for AAT principals: an AAT with
	// role=admin and a project binding cannot reach a different
	// project. This test pins the helper that callers use to make
	// that decision.
	aat := &api.Principal{
		Kind:      api.PrincipalAATKind,
		Role:      user.RoleAdmin,
		ProjectID: "p1",
	}
	if aat.IsGlobalAdmin() {
		t.Error("project-bound AAT.IsGlobalAdmin() = true, want false")
	}

	global := &api.Principal{
		Kind:      api.PrincipalAATKind,
		Role:      user.RoleAdmin,
		ProjectID: "",
	}
	if !global.IsGlobalAdmin() {
		t.Error("unbound AAT.IsGlobalAdmin() = false, want true")
	}

	human := &api.Principal{
		Kind: api.PrincipalUser,
		Role: user.RoleAdmin,
	}
	if !human.IsGlobalAdmin() {
		t.Error("human admin.IsGlobalAdmin() = false, want true")
	}
}
