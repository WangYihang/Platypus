package api

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// M4: project-bound AATs must NOT pass RequireGlobalRole. By choosing
// to bind an AAT to a specific project the issuer scoped the token
// out of any global resource, regardless of the role they stamped on
// it. Two pieces of context that make this non-obvious:
//
//	· Principal.IsGlobalAdmin() already does the right thing for
//	  global-admin bypasses inside RequireProjectRole — but
//	  RequireGlobalRole reads p.Role directly via roleAtLeast, so a
//	  project-bound role=admin AAT used to satisfy a global admin gate.
//	· claimsForPrincipal() in rbac.go floors the legacy AccessClaims
//	  role to viewer for project-bound AATs, but that only protects
//	  handlers that read claims (legacy path); handlers that read
//	  Principal directly were exposed.
//
// Test mounts a synthetic /admin route guarded by RequireAuth +
// RequireGlobalRole(admin) so the assertion is exactly "global gate
// rejects project-bound credentials" without depending on any real
// admin endpoint shape.
func mountGlobalAdminRoute(rbac *RBAC) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin",
		rbac.RequireAuth(),
		rbac.RequireGlobalRole(user.RoleAdmin),
		func(c *gin.Context) { c.String(http.StatusOK, "ok") },
	)
	return r
}

func TestRequireGlobalRole_AAT_Bound_AdminRejected(t *testing.T) {
	f := aatRBACSetup(t)
	r := mountGlobalAdminRoute(f.rbac)

	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p1 := seedProjectForAPITest(t, f.db, "p1", owner)
	// Project-bound admin-role AAT — the credential the issuer said
	// should only act inside p1.
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = p1.ID
		x.Role = user.RoleAdmin
	})

	w := probeReqWithPath(r, "GET", "/admin", raw, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("project-bound admin AAT against global-admin route: status=%d body=%s; want 403",
			w.Code, w.Body.String())
	}
}

func TestRequireGlobalRole_AAT_Bound_OperatorRejected(t *testing.T) {
	f := aatRBACSetup(t)
	r := mountGlobalAdminRoute(f.rbac)

	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p1 := seedProjectForAPITest(t, f.db, "p1", owner)
	// Project-bound operator AAT should also be 403 — both because
	// "operator < admin" AND because project-bound is itself
	// disqualifying. Either reason is sufficient.
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = p1.ID
		x.Role = user.RoleOperator
	})

	w := probeReqWithPath(r, "GET", "/admin", raw, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("project-bound operator AAT: status=%d, want 403", w.Code)
	}
}

func TestRequireGlobalRole_AAT_Unbound_AdminAllowed(t *testing.T) {
	f := aatRBACSetup(t)
	r := mountGlobalAdminRoute(f.rbac)

	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	// Unbound (global) admin AAT — the platform-admin equivalent of
	// a service token. Passes.
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = ""
		x.Role = user.RoleAdmin
	})

	w := probeReqWithPath(r, "GET", "/admin", raw, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("unbound admin AAT against global-admin route: status=%d body=%s; want 200",
			w.Code, w.Body.String())
	}
}

func TestRequireGlobalRole_AAT_Bound_AdminInOwnProjectStillWorks(t *testing.T) {
	// Sanity check that the M4 fix doesn't accidentally also revoke
	// admin powers for a project-bound admin AAT WITHIN its bound
	// project. RequireProjectRole has its own AAT branch and should
	// still let this through.
	f := aatRBACSetup(t)

	owner := seedUserForAPITest(t, f.db, "owner", user.RoleAdmin)
	p1 := seedProjectForAPITest(t, f.db, "p1", owner)
	raw, _ := seedAAT(t, f.db, func(x *storage.AAT) {
		x.UserID = owner.ID
		x.ProjectID = p1.ID
		x.Role = user.RoleAdmin
	})

	// /projects/:pid/probe is mounted with RequireProjectRole(viewer),
	// which any role passes. Use that as proof the credential is still
	// authentic — we just want to confirm it isn't being globally
	// blocked.
	w := probeReqWithPath(f.router, "GET", "/projects/"+p1.ID+"/probe", raw, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("project-bound admin AAT in own project: status=%d body=%s; want 200",
			w.Code, w.Body.String())
	}
}
