package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func projectRBACSetup(t *testing.T) (*gin.Engine, *storage.DB, *TokenIssuer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	issuer, _ := NewTokenIssuer("a", "b", 5*time.Minute, time.Hour)
	rbac := NewRBACWithStorage(issuer, db)

	r := gin.New()
	// /projects/:pid/probe is gated by RequireAuth + RequireProjectRole.
	r.GET("/projects/:pid/probe",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleOperator),
		func(c *gin.Context) { c.String(http.StatusOK, "ok") },
	)
	return r, db, issuer
}

func TestRequireProjectRole_AdminAlwaysPasses(t *testing.T) {
	r, db, issuer := projectRBACSetup(t)
	// Create a project owned by someone else; admin is not a member.
	owner := seedUserForAPITest(t, db, "owner", user.RoleAdmin)
	p := seedProjectForAPITest(t, db, "prod", owner)

	// A global admin who is NOT in project_members still gets through.
	tok, _ := issuer.IssueAccess(AccessClaims{
		UserID: "someone-else", Role: user.RoleAdmin,
	})
	w := probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("admin status=%d; want 200", w.Code)
	}
}

func TestRequireProjectRole_NonMemberBlocked(t *testing.T) {
	r, db, issuer := projectRBACSetup(t)
	owner := seedUserForAPITest(t, db, "owner", user.RoleAdmin)
	p := seedProjectForAPITest(t, db, "prod", owner)

	// Non-admin, no membership → 403.
	tok, _ := issuer.IssueAccess(AccessClaims{
		UserID: "stranger", Role: user.RoleOperator,
	})
	w := probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("stranger status=%d; want 403", w.Code)
	}
}

func TestRequireProjectRole_MemberRoleChecked(t *testing.T) {
	r, db, issuer := projectRBACSetup(t)
	owner := seedUserForAPITest(t, db, "owner", user.RoleAdmin)
	bob := seedUserForAPITest(t, db, "bob", user.RoleViewer)
	carol := seedUserForAPITest(t, db, "carol", user.RoleViewer)
	p := seedProjectForAPITest(t, db, "prod", owner)

	// Bob is a viewer on the project; route requires operator → 403.
	_ = db.Projects().AddMember(context.Background(), p.ID, bob.ID, user.RoleViewer)
	// Carol is an operator → passes.
	_ = db.Projects().AddMember(context.Background(), p.ID, carol.ID, user.RoleOperator)

	bobTok, _ := issuer.IssueAccess(AccessClaims{UserID: bob.ID, Role: user.RoleViewer})
	w := probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", bobTok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("bob (viewer on operator route) status=%d; want 403", w.Code)
	}

	carolTok, _ := issuer.IssueAccess(AccessClaims{UserID: carol.ID, Role: user.RoleViewer})
	w = probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", carolTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("carol (operator) status=%d; want 200", w.Code)
	}
}

// Non-admin hitting a missing project must get 403, not 404 — returning
// 404 would leak which project ids exist to unauthorized callers. Admins
// get the honest 404 (covered by a separate assertion below).
func TestRequireProjectRole_UnknownProject_NonAdmin403(t *testing.T) {
	r, db, issuer := projectRBACSetup(t)
	bob := seedUserForAPITest(t, db, "bob", user.RoleOperator)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: bob.ID, Role: user.RoleOperator})
	w := probeReqWithPath(r, "GET", "/projects/does-not-exist/probe", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin missing-project status=%d; want 403", w.Code)
	}
}

func TestRequireProjectRole_UnknownProject_Admin404(t *testing.T) {
	r, _, issuer := projectRBACSetup(t)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: "admin-u", Role: user.RoleAdmin})

	w := probeReqWithPath(r, "GET", "/projects/does-not-exist/probe", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("admin missing-project status=%d; want 404", w.Code)
	}
}

// Local test helpers that avoid importing storage_test (cross-package).

func seedUserForAPITest(t *testing.T, db *storage.DB, username string, role user.Role) *user.User {
	t.Helper()
	u := &user.User{
		ID:           "u-" + username,
		Username:     username,
		PasswordHash: "x",
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}
	if err := db.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("seed user %q: %v", username, err)
	}
	return u
}

func seedProjectForAPITest(t *testing.T, db *storage.DB, slug string, creator *user.User) *storage.Project {
	t.Helper()
	p := &storage.Project{
		ID:        "prj-" + slug,
		Name:      slug,
		Slug:      slug,
		CreatedAt: time.Now().UTC(),
		CreatedBy: creator.ID,
	}
	if err := db.Projects().Create(context.Background(), p); err != nil {
		t.Fatalf("seed project %q: %v", slug, err)
	}
	return p
}
