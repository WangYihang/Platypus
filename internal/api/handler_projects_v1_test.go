package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func projectsTestSetup(t *testing.T) (*gin.Engine, *storage.DB) {
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
	h := NewProjectsHandler(db)

	r := gin.New()
	RegisterV1ProjectsRoutes(r, h, rbac)
	return r, db
}

type projectBody struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedBy string `json:"created_by"`
}

func TestProjects_AdminCreatesAndLists(t *testing.T) {
	r, db := projectsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	w := probeReqWithPath(r, "POST", "/api/v1/projects", tok, map[string]string{
		"name": "Production", "slug": "prod",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", w.Code, w.Body.String())
	}

	w = probeReqWithPath(r, "GET", "/api/v1/projects", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Projects []projectBody `json:"projects"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Projects) != 1 || resp.Projects[0].Slug != "prod" {
		t.Fatalf("listed: %+v", resp.Projects)
	}
}

func TestProjects_CreateForbiddenForNonAdmin(t *testing.T) {
	r, db := projectsTestSetup(t)
	bob := seedUserForAPITest(t, db, "bob", user.RoleOperator)
	tok := mintBearerForUserID(t, db, bob.ID, user.RoleOperator)

	w := probeReqWithPath(r, "POST", "/api/v1/projects", tok, map[string]string{
		"name": "Other", "slug": "other",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d; want 403", w.Code)
	}
}

func TestProjects_ListForUserFiltersByMembership(t *testing.T) {
	r, db := projectsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	bob := seedUserForAPITest(t, db, "bob", user.RoleOperator)

	// Admin creates two projects, adds bob to only one.
	prod := seedProjectForAPITest(t, db, "prod", admin)
	seedProjectForAPITest(t, db, "staging", admin)
	_ = db.Projects().AddMember(testCtx(), prod.ID, bob.ID, user.RoleOperator)

	bobTok := mintBearerForUserID(t, db, bob.ID, user.RoleOperator)
	w := probeReqWithPath(r, "GET", "/api/v1/projects", bobTok, nil)
	var resp struct {
		Projects []projectBody `json:"projects"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Projects) != 1 || resp.Projects[0].Slug != "prod" {
		t.Fatalf("bob sees: %+v; want [prod]", resp.Projects)
	}
}

func TestProjects_DeleteAdminOnly(t *testing.T) {
	r, db := projectsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	bob := seedUserForAPITest(t, db, "bob", user.RoleOperator)
	p := seedProjectForAPITest(t, db, "prod", admin)

	bobTok := mintBearerForUserID(t, db, bob.ID, user.RoleOperator)
	w := probeReqWithPath(r, "DELETE", "/api/v1/projects/"+p.ID, bobTok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("bob delete status=%d; want 403", w.Code)
	}

	adminTok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w = probeReqWithPath(r, "DELETE", "/api/v1/projects/"+p.ID, adminTok, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("admin delete status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestProjects_MemberManagement(t *testing.T) {
	r, db := projectsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	bob := seedUserForAPITest(t, db, "bob", user.RoleOperator)
	p := seedProjectForAPITest(t, db, "prod", admin)

	adminTok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	// Add bob as operator.
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+p.ID+"/members", adminTok,
		map[string]string{"user_id": bob.ID, "role": "operator"})
	if w.Code != http.StatusNoContent {
		t.Fatalf("add member status=%d body=%s", w.Code, w.Body.String())
	}

	// List members includes bob.
	w = probeReqWithPath(r, "GET", "/api/v1/projects/"+p.ID+"/members", adminTok, nil)
	var resp struct {
		Members []struct {
			UserID   string `json:"user_id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"members"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Members) != 1 || resp.Members[0].UserID != bob.ID || resp.Members[0].Role != "operator" {
		t.Fatalf("members: %+v", resp.Members)
	}

	// Remove bob.
	w = probeReqWithPath(r, "DELETE", "/api/v1/projects/"+p.ID+"/members/"+bob.ID, adminTok, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("remove status=%d body=%s", w.Code, w.Body.String())
	}
}

// A project admin (not global) can also manage members.
func TestProjects_ProjectAdminCanManageMembers(t *testing.T) {
	r, db := projectsTestSetup(t)
	globalAdmin := seedUserForAPITest(t, db, "root", user.RoleAdmin)
	alice := seedUserForAPITest(t, db, "alice", user.RoleOperator)
	bob := seedUserForAPITest(t, db, "bob", user.RoleOperator)
	p := seedProjectForAPITest(t, db, "prod", globalAdmin)

	// Make alice a project admin.
	_ = db.Projects().AddMember(testCtx(), p.ID, alice.ID, user.RoleAdmin)

	aliceTok := mintBearerForUserID(t, db, alice.ID, user.RoleOperator)
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+p.ID+"/members", aliceTok,
		map[string]string{"user_id": bob.ID, "role": "viewer"})
	if w.Code != http.StatusNoContent {
		t.Fatalf("project-admin add member status=%d body=%s", w.Code, w.Body.String())
	}
}
