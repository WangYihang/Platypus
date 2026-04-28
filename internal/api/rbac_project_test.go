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

func projectRBACSetup(t *testing.T) (*gin.Engine, *storage.DB) {
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
	// /projects/:pid/probe is gated by RequireAuth + RequireProjectRole.
	r.GET("/projects/:pid/probe",
		rbac.RequireAuth(),
		rbac.RequireProjectRole("pid", user.RoleOperator),
		func(c *gin.Context) { c.String(http.StatusOK, "ok") },
	)
	return r, db
}

func TestRequireProjectRole_AdminAlwaysPasses(t *testing.T) {
	r, db := projectRBACSetup(t)
	// Create a project owned by someone else; admin is not a member.
	owner := seedUserForAPITest(t, db, "owner", user.RoleAdmin)
	p := seedProjectForAPITest(t, db, "prod", owner)

	// A global admin who is NOT in project_members still gets through.
	tok := mintBearerForUserID(t, db, "someone-else", user.RoleAdmin)
	w := probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("admin status=%d; want 200", w.Code)
	}
}

func TestRequireProjectRole_NonMemberBlocked(t *testing.T) {
	r, db := projectRBACSetup(t)
	owner := seedUserForAPITest(t, db, "owner", user.RoleAdmin)
	p := seedProjectForAPITest(t, db, "prod", owner)

	// Non-admin, no membership → 403.
	tok := mintBearerForUserID(t, db, "stranger", user.RoleOperator)
	w := probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("stranger status=%d; want 403", w.Code)
	}
}

func TestRequireProjectRole_MemberRoleChecked(t *testing.T) {
	r, db := projectRBACSetup(t)
	owner := seedUserForAPITest(t, db, "owner", user.RoleAdmin)
	bob := seedUserForAPITest(t, db, "bob", user.RoleViewer)
	carol := seedUserForAPITest(t, db, "carol", user.RoleViewer)
	p := seedProjectForAPITest(t, db, "prod", owner)

	// Bob is a viewer on the project; route requires operator → 403.
	_ = db.Projects().AddMember(context.Background(), p.ID, bob.ID, user.RoleViewer)
	// Carol is an operator on the project → passes.
	_ = db.Projects().AddMember(context.Background(), p.ID, carol.ID, user.RoleOperator)

	bobTok := mintBearerForUserID(t, db, bob.ID, user.RoleViewer)
	w := probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", bobTok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("bob (viewer on operator route) status=%d; want 403", w.Code)
	}

	carolTok := mintBearerForUserID(t, db, carol.ID, user.RoleViewer)
	w = probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", carolTok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("carol (operator) status=%d; want 200", w.Code)
	}
}

// Non-admin hitting a missing project must get 403, not 404 — returning
// 404 would leak which project ids exist to unauthorized callers. Admins
// get the honest 404 (covered by a separate assertion below).
func TestRequireProjectRole_UnknownProject_NonAdmin403(t *testing.T) {
	r, db := projectRBACSetup(t)
	bob := seedUserForAPITest(t, db, "bob", user.RoleOperator)

	tok := mintBearerForUserID(t, db, bob.ID, user.RoleOperator)
	w := probeReqWithPath(r, "GET", "/projects/does-not-exist/probe", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin missing-project status=%d; want 403", w.Code)
	}
}

func TestRequireProjectRole_UnknownProject_Admin404(t *testing.T) {
	r, db := projectRBACSetup(t)
	tok := mintBearerForUserID(t, db, "admin-u", user.RoleAdmin)

	w := probeReqWithPath(r, "GET", "/projects/does-not-exist/probe", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("admin missing-project status=%d; want 404", w.Code)
	}
}

// TestRequireProjectRole_CustomRoleWithOperatorPerms locks in the
// new RBAC contract: a user whose project_members.role is a CUSTOM
// role (not in the builtin viewer<operator<admin chain) passes
// RequireProjectRole(operator) iff its permission set is a superset
// of the builtin operator role's permissions. This is the
// "permission-superset, not enum hierarchy" rule that lets an admin
// design custom roles like "incident-responder" without rewriting
// every gated route.
func TestRequireProjectRole_CustomRoleWithOperatorPerms(t *testing.T) {
	r, db := projectRBACSetup(t)
	owner := seedUserForAPITest(t, db, "owner", user.RoleAdmin)
	donna := seedUserForAPITest(t, db, "donna", user.RoleViewer)
	earl := seedUserForAPITest(t, db, "earl", user.RoleViewer)
	p := seedProjectForAPITest(t, db, "prod", owner)

	// Pull operator's seeded permissions so we can mirror them.
	opRole, err := db.Roles().Get(context.Background(), "operator")
	if err != nil {
		t.Fatalf("Roles.Get operator: %v", err)
	}

	// "responder": superset of operator's perms (passes).
	now := time.Now().UTC()
	if err := db.Roles().Create(context.Background(),
		&storage.Role{
			Slug:      "responder",
			Name:      "Incident Responder",
			IsProject: true,
			IsGlobal:  false,
			CreatedAt: now,
			UpdatedAt: now,
		},
		opRole.Permissions,
	); err != nil {
		t.Fatalf("Roles.Create responder: %v", err)
	}

	// "support": missing rpc:invoke from operator's set (fails).
	missing := []string{}
	for _, perm := range opRole.Permissions {
		if perm != optoken.ScopeRPCInvoke {
			missing = append(missing, perm)
		}
	}
	if err := db.Roles().Create(context.Background(),
		&storage.Role{
			Slug:      "support",
			Name:      "Support",
			IsProject: true,
			IsGlobal:  false,
			CreatedAt: now,
			UpdatedAt: now,
		},
		missing,
	); err != nil {
		t.Fatalf("Roles.Create support: %v", err)
	}

	_ = db.Projects().AddMember(context.Background(), p.ID, donna.ID, user.Role("responder"))
	_ = db.Projects().AddMember(context.Background(), p.ID, earl.ID, user.Role("support"))

	donnaTok := mintBearerForUserID(t, db, donna.ID, user.RoleViewer)
	w := probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", donnaTok, nil)
	if w.Code != http.StatusOK {
		t.Errorf("donna (responder ⊇ operator) status=%d; want 200", w.Code)
	}

	earlTok := mintBearerForUserID(t, db, earl.ID, user.RoleViewer)
	w = probeReqWithPath(r, "GET", "/projects/"+p.ID+"/probe", earlTok, nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("earl (support missing rpc:invoke) status=%d; want 403", w.Code)
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
