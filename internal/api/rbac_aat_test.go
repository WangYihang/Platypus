package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// aatRBACSetup builds an RBAC wired with an AAT-aware verifier and an
// in-memory db.
type aatFixture struct {
	router *gin.Engine
	db     *storage.DB
	rbac   *RBAC
	cache  *optoken.Cache
}

func aatRBACSetup(t *testing.T) *aatFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBAC(db, verifier)

	r := gin.New()
	// Two probes: one global (RequireAuth only), one project-scoped.
	r.GET("/probe",
		rbac.RequireAuth(),
		func(c *gin.Context) {
			p, _ := PrincipalFromContext(c)
			c.JSON(http.StatusOK, gin.H{
				"kind":     int(p.Kind),
				"user_id":  p.UserID,
				"token_id": p.TokenID,
				"role":     string(p.Role),
			})
		},
	)
	r.GET("/projects/:pid/probe",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleViewer),
		func(c *gin.Context) { c.String(http.StatusOK, "ok") },
	)
	r.GET("/projects/:pid/exec",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleViewer),
		rbac.RequireScope(optoken.ScopeHostsExec),
		func(c *gin.Context) { c.String(http.StatusOK, "exec-ok") },
	)
	r.GET("/projects/:pid/read",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleViewer),
		rbac.RequireScope(optoken.ScopeHostsRead),
		func(c *gin.Context) { c.String(http.StatusOK, "read-ok") },
	)
	return &aatFixture{router: r, db: db, rbac: rbac, cache: cache}
}

// seedAAT mints an AAT row and returns its plaintext bearer.
func seedAAT(t *testing.T, db *storage.DB, opts func(*storage.AAT)) (raw string, a *storage.AAT) {
	t.Helper()
	id, _, hash, plaintext, err := optoken.Generate(optoken.AATPrefix)
	if err != nil {
		t.Fatalf("optoken.Generate: %v", err)
	}
	a = &storage.AAT{
		TokenID:    id,
		SecretHash: hash,
		UserID:     "u-issuer",
		Name:       "test",
		Role:       user.RoleOperator,
		Scopes:     []string{optoken.ScopeHostsRead},
		CreatedAt:  time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	}
	if opts != nil {
		opts(a)
	}
	if err := db.AuthTokens().CreateAAT(context.Background(), a); err != nil {
		t.Fatalf("CreateAAT: %v", err)
	}
	return plaintext, a
}

// ---- RequireAuth ----

func TestRequireAuth_AAT_Success(t *testing.T) {
	f := aatRBACSetup(t)
	seedUserForAPITest(t, f.db, "issuer", user.RoleAdmin)
	raw, a := seedAAT(t, f.db, func(x *storage.AAT) { x.UserID = "u-issuer" })

	w := probeReqWithPath(f.router, "GET", "/probe", raw, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), a.TokenID) {
		t.Errorf("response missing token_id %q: %s", a.TokenID, w.Body.String())
	}
}

func TestRequireAuth_AAT_Revoked(t *testing.T) {
	f := aatRBACSetup(t)
	seedUserForAPITest(t, f.db, "issuer", user.RoleAdmin)
	raw, a := seedAAT(t, f.db, func(x *storage.AAT) { x.UserID = "u-issuer" })
	if err := f.db.AuthTokens().Revoke(context.Background(), a.TokenID, "u-issuer", "test", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	w := probeReqWithPath(f.router, "GET", "/probe", raw, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("revoked AAT status=%d, want 401", w.Code)
	}
}

func TestRequireAuth_Session(t *testing.T) {
	f := aatRBACSetup(t)
	u := seedUserForAPITest(t, f.db, "u1", user.RoleViewer)
	tok := mintSessionForTest(t, f.db, u)
	w := probeReqWithPath(f.router, "GET", "/probe", tok, nil)
	if w.Code != http.StatusOK {
		t.Errorf("session-path: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRequireAuth_GarbageToken(t *testing.T) {
	f := aatRBACSetup(t)
	w := probeReqWithPath(f.router, "GET", "/probe", "garbage-not-jwt-not-aat", nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("garbage token status=%d, want 401", w.Code)
	}
}

// ---- RequireProjectRole with AAT ----

func TestRequireProjectRole_AAT_Bound_PassesOwnProject(t *testing.T) {
	f := aatRBACSetup(t)
	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p := seedProjectForAPITest(t, f.db, "p1", owner)
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = p.ID
	})
	w := probeReqWithPath(f.router, "GET", "/projects/"+p.ID+"/probe", raw, nil)
	if w.Code != http.StatusOK {
		t.Errorf("status=%d body=%s; want 200", w.Code, w.Body.String())
	}
}

func TestRequireProjectRole_AAT_Bound_RejectsOtherProject(t *testing.T) {
	f := aatRBACSetup(t)
	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p1 := seedProjectForAPITest(t, f.db, "p1", owner)
	p2 := seedProjectForAPITest(t, f.db, "p2", owner)
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = p1.ID
	})
	w := probeReqWithPath(f.router, "GET", "/projects/"+p2.ID+"/probe", raw, nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("AAT bound to %s reaching %s: status=%d, want 403", p1.ID, p2.ID, w.Code)
	}
}

func TestRequireProjectRole_AAT_Bound_AdminCannotEscape(t *testing.T) {
	f := aatRBACSetup(t)
	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p1 := seedProjectForAPITest(t, f.db, "p1", owner)
	p2 := seedProjectForAPITest(t, f.db, "p2", owner)
	// Admin-role AAT bound to p1: must NOT bypass into p2.
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = p1.ID
		x.Role = user.RoleAdmin
	})
	w := probeReqWithPath(f.router, "GET", "/projects/"+p2.ID+"/probe", raw, nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("admin-AAT bound to p1 reaching p2: status=%d, want 403 (binding must trump role)", w.Code)
	}
}

func TestRequireProjectRole_AAT_Global_AdminBypasses(t *testing.T) {
	f := aatRBACSetup(t)
	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p1 := seedProjectForAPITest(t, f.db, "p1", owner)
	// Unbound (global) AAT with admin role: this is the platform-admin
	// equivalent — passes the project gate without a member row.
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = ""
		x.Role = user.RoleAdmin
	})
	w := probeReqWithPath(f.router, "GET", "/projects/"+p1.ID+"/probe", raw, nil)
	if w.Code != http.StatusOK {
		t.Errorf("global admin AAT: status=%d, want 200", w.Code)
	}
}

// ---- RequireScope ----

func TestRequireScope_AAT_Allow(t *testing.T) {
	f := aatRBACSetup(t)
	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p := seedProjectForAPITest(t, f.db, "p1", owner)
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = p.ID
		x.Scopes = []string{optoken.ScopeHostsRead, optoken.ScopeHostsExec}
	})
	w := probeReqWithPath(f.router, "GET", "/projects/"+p.ID+"/exec", raw, nil)
	if w.Code != http.StatusOK {
		t.Errorf("AAT with hosts:exec scope: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestRequireScope_AAT_Deny(t *testing.T) {
	f := aatRBACSetup(t)
	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p := seedProjectForAPITest(t, f.db, "p1", owner)
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = p.ID
		x.Scopes = []string{optoken.ScopeHostsRead} // no exec
	})
	w := probeReqWithPath(f.router, "GET", "/projects/"+p.ID+"/exec", raw, nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("AAT lacking hosts:exec: status=%d, want 403", w.Code)
	}
}

func TestRequireScope_HumanOperator_AlwaysPass(t *testing.T) {
	f := aatRBACSetup(t)
	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p := seedProjectForAPITest(t, f.db, "p1", owner)
	bob := seedUserForAPITest(t, f.db, "bob", user.RoleOperator)
	_ = f.db.Projects().AddMember(context.Background(), p.ID, bob.ID, user.RoleOperator)

	tok := mintBearerForUserID(t, f.db, bob.ID, user.RoleOperator)
	w := probeReqWithPath(f.router, "GET", "/projects/"+p.ID+"/exec", tok, nil)
	if w.Code != http.StatusOK {
		t.Errorf("operator on /exec: status=%d body=%s; humans should pass scope checks via ScopesFromRole",
			w.Code, w.Body.String())
	}
}

func TestRequireScope_HumanViewer_DeniedExec(t *testing.T) {
	f := aatRBACSetup(t)
	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p := seedProjectForAPITest(t, f.db, "p1", owner)
	bob := seedUserForAPITest(t, f.db, "bob", user.RoleViewer)
	_ = f.db.Projects().AddMember(context.Background(), p.ID, bob.ID, user.RoleViewer)

	tok := mintBearerForUserID(t, f.db, bob.ID, user.RoleViewer)
	w := probeReqWithPath(f.router, "GET", "/projects/"+p.ID+"/exec", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("viewer on /exec: status=%d; viewers must not pass hosts:exec", w.Code)
	}
	w = probeReqWithPath(f.router, "GET", "/projects/"+p.ID+"/read", tok, nil)
	if w.Code != http.StatusOK {
		t.Errorf("viewer on /read: status=%d; viewers should pass hosts:read", w.Code)
	}
}

// ---- helpers ----

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
