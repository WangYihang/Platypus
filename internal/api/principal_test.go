package api_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/user"
)

// PrincipalFromClaims was removed alongside the JWT path. The
// equivalent test now exercises PrincipalFromVerified for a
// user_session-kind Verified — that's the production constructor for
// human principals after the AAT removal.
func TestPrincipalFromVerified_UserSession(t *testing.T) {
	t.Parallel()
	v := &optoken.Verified{
		TokenID:  "pst_alice",
		Kind:     optoken.KindUserSession,
		UserID:   "u-alice",
		Username: "alice",
		Role:     user.RoleOperator,
		Scopes:   optoken.ScopesFromRole(user.RoleOperator),
	}
	p := api.PrincipalFromVerified(v)
	if p.Kind != api.PrincipalUser {
		t.Errorf("Kind = %v, want PrincipalUser", p.Kind)
	}
	if p.UserID != "u-alice" || p.Username != "alice" || p.Role != user.RoleOperator {
		t.Errorf("identity mismatch: %+v", p)
	}
	if !optoken.HasScope(p.Scopes, optoken.ScopeHostsRead) {
		t.Errorf("operator scopes missing hosts:read: %v", p.Scopes)
	}
	if p.ProjectID != "" {
		t.Errorf("user_session principal should leave ProjectID empty: %+v", p)
	}
	if p.TokenID != "pst_alice" {
		t.Errorf("TokenID = %q, want pst_alice (audit join key)", p.TokenID)
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
		Kind:    api.PrincipalUser,
		UserID:  "u-round",
		TokenID: "pst_round",
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

func TestPrincipal_HumanAdminIsGlobalAdmin(t *testing.T) {
	t.Parallel()
	human := &api.Principal{
		Kind: api.PrincipalUser,
		Role: user.RoleAdmin,
	}
	if !human.IsGlobalAdmin() {
		t.Error("human admin.IsGlobalAdmin() = false, want true")
	}
	viewer := &api.Principal{
		Kind: api.PrincipalUser,
		Role: user.RoleViewer,
	}
	if viewer.IsGlobalAdmin() {
		t.Error("human viewer.IsGlobalAdmin() = true, want false")
	}
}
