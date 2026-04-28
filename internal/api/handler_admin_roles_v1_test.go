package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// adminRolesSetup mounts both /api/v1/admin/permissions (read) and
// /api/v1/admin/roles CRUD behind RequireGlobalRole(admin), the same
// gate the other admin-only routes use.
func adminRolesSetup(t *testing.T) (*gin.Engine, *storage.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := api.NewTokenVerifier(db, cache)
	rbac := api.NewRBAC(db, verifier)
	h := api.NewAdminRolesHandler(db)

	r := gin.New()
	api.RegisterV1AdminRolesRoutes(r, h, rbac)
	return r, db
}

// adminBearer mints a session for an admin user and returns the
// plaintext token.
func adminBearer(t *testing.T, db *storage.DB, name string) string {
	t.Helper()
	hash, _ := user.HashPassword("pw")
	u := &user.User{
		ID: uuid.NewString(), Username: name,
		PasswordHash: hash, Role: user.RoleAdmin,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("Create admin: %v", err)
	}
	id, _, secretHash, plaintext, err := optoken.Generate(optoken.UserSessionPrefix)
	if err != nil {
		t.Fatalf("Generate session: %v", err)
	}
	now := time.Now().UTC()
	s := &storage.UserSession{
		TokenID: id, SecretHash: secretHash, UserID: u.ID,
		CreatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour),
		IdleExpiresAt: now.Add(time.Hour),
	}
	if err := db.AuthTokens().CreateSession(context.Background(), s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return plaintext
}

func viewerBearer(t *testing.T, db *storage.DB, name string) string {
	t.Helper()
	hash, _ := user.HashPassword("pw")
	u := &user.User{
		ID: uuid.NewString(), Username: name,
		PasswordHash: hash, Role: user.RoleViewer,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("Create viewer: %v", err)
	}
	id, _, secretHash, plaintext, err := optoken.Generate(optoken.UserSessionPrefix)
	if err != nil {
		t.Fatalf("Generate session: %v", err)
	}
	now := time.Now().UTC()
	s := &storage.UserSession{
		TokenID: id, SecretHash: secretHash, UserID: u.ID,
		CreatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour),
		IdleExpiresAt: now.Add(time.Hour),
	}
	if err := db.AuthTokens().CreateSession(context.Background(), s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return plaintext
}

func adminReq(t *testing.T, r *gin.Engine, method, path, bearer string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAdminRoles_RequireAdmin(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := viewerBearer(t, db, "viewer-x")

	for _, p := range []struct{ method, path string }{
		{"GET", "/api/v1/admin/permissions"},
		{"GET", "/api/v1/admin/roles"},
		{"POST", "/api/v1/admin/roles"},
		{"GET", "/api/v1/admin/roles/viewer"},
		{"PATCH", "/api/v1/admin/roles/viewer"},
		{"DELETE", "/api/v1/admin/roles/viewer"},
	} {
		w := adminReq(t, r, p.method, p.path, tok, nil)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s as viewer = %d, want 403", p.method, p.path, w.Code)
		}
	}
}

func TestAdminRoles_ListPermissions(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-list-perm")

	w := adminReq(t, r, "GET", "/api/v1/admin/permissions", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Permissions []struct {
			Slug, Resource, Description string
		} `json:"permissions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Permissions) < 17 {
		t.Errorf("expected ≥17 catalogue rows, got %d", len(resp.Permissions))
	}
	for _, p := range resp.Permissions {
		if p.Slug == "" || p.Resource == "" || p.Description == "" {
			t.Errorf("incomplete permission row: %+v", p)
		}
	}
}

func TestAdminRoles_ListRoles(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-list-roles")

	w := adminReq(t, r, "GET", "/api/v1/admin/roles", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Roles []struct {
			Slug      string `json:"slug"`
			Name      string `json:"name"`
			IsBuiltin bool   `json:"is_builtin"`
		} `json:"roles"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	bySlug := map[string]bool{}
	for _, role := range resp.Roles {
		bySlug[role.Slug] = role.IsBuiltin
	}
	for _, want := range []string{"viewer", "operator", "admin"} {
		if !bySlug[want] {
			t.Errorf("builtin %q missing or not flagged is_builtin: %v", want, bySlug)
		}
	}
}

func TestAdminRoles_GetRole_PopulatesPermissions(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-get-role")

	w := adminReq(t, r, "GET", "/api/v1/admin/roles/viewer", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Slug        string   `json:"slug"`
		Permissions []string `json:"permissions"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Slug != "viewer" {
		t.Errorf("slug=%q want viewer", resp.Slug)
	}
	if len(resp.Permissions) == 0 {
		t.Error("permissions empty for viewer role")
	}
}

func TestAdminRoles_GetRole_NotFound(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-404")
	w := adminReq(t, r, "GET", "/api/v1/admin/roles/no-such", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d want 404", w.Code)
	}
}

func TestAdminRoles_CreateCustomRole(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-create-role")

	body := map[string]any{
		"slug":        "responder",
		"name":        "Incident Responder",
		"description": "Time-bound operator role granted during oncall.",
		"is_global":   false,
		"is_project":  true,
		"permissions": []string{"hosts:read", "hosts:exec"},
	}
	w := adminReq(t, r, "POST", "/api/v1/admin/roles", tok, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}

	// Round-trip with GET to confirm.
	w = adminReq(t, r, "GET", "/api/v1/admin/roles/responder", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("post-create get status=%d", w.Code)
	}
	var resp struct {
		Slug        string   `json:"slug"`
		IsBuiltin   bool     `json:"is_builtin"`
		IsGlobal    bool     `json:"is_global"`
		IsProject   bool     `json:"is_project"`
		Permissions []string `json:"permissions"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.IsBuiltin || resp.IsGlobal || !resp.IsProject {
		t.Errorf("flag mismatch: %+v", resp)
	}
	if len(resp.Permissions) != 2 {
		t.Errorf("permissions count = %d want 2", len(resp.Permissions))
	}
}

func TestAdminRoles_Create_RejectsBadInput(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-bad")

	cases := []map[string]any{
		{ // empty slug
			"slug": "", "name": "X", "is_project": true,
			"permissions": []string{"hosts:read"},
		},
		{ // unknown permission
			"slug": "ghost", "name": "Ghost", "is_project": true,
			"permissions": []string{"does:not:exist"},
		},
		{ // neither global nor project
			"slug": "limbo", "name": "Limbo", "is_global": false, "is_project": false,
			"permissions": []string{"hosts:read"},
		},
	}
	for i, c := range cases {
		w := adminReq(t, r, "POST", "/api/v1/admin/roles", tok, c)
		if w.Code != http.StatusBadRequest {
			t.Errorf("case %d status=%d body=%s; want 400", i, w.Code, w.Body.String())
		}
	}
}

func TestAdminRoles_UpdateBuiltinPermissions(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-edit-builtin")

	// Add enrollment:issue to viewer (a real ops case: "let viewers
	// hand out install links").
	w := adminReq(t, r, "PATCH", "/api/v1/admin/roles/viewer", tok, map[string]any{
		"permissions": []string{
			"hosts:read", "files:read", "projects:read", "activity:read", "enrollment:issue",
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	got, _ := db.Roles().Get(context.Background(), "viewer")
	found := false
	for _, p := range got.Permissions {
		if p == "enrollment:issue" {
			found = true
		}
	}
	if !found {
		t.Errorf("viewer permissions missing enrollment:issue: %v", got.Permissions)
	}
}

func TestAdminRoles_Update_AdminProtect(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-self-strip")

	// Try to drop admin:roles from admin role; the DB trigger aborts.
	w := adminReq(t, r, "PATCH", "/api/v1/admin/roles/admin", tok, map[string]any{
		"permissions": []string{"hosts:read", "hosts:exec"},
	})
	if w.Code == http.StatusOK {
		t.Errorf("PATCH admin role without admin:* succeeded; trigger should have aborted")
	}
}

func TestAdminRoles_Delete_BuiltinRejected(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-del-builtin")

	w := adminReq(t, r, "DELETE", "/api/v1/admin/roles/viewer", tok, nil)
	if w.Code != http.StatusConflict {
		t.Errorf("DELETE builtin status=%d want 409", w.Code)
	}
}

func TestAdminRoles_Delete_InUseRejectedThenAllowed(t *testing.T) {
	t.Parallel()
	r, db := adminRolesSetup(t)
	tok := adminBearer(t, db, "admin-del-inuse")
	ctx := context.Background()

	now := time.Now().UTC()
	if err := db.Roles().Create(ctx,
		&storage.Role{
			Slug: "support", Name: "Support",
			IsGlobal: true, IsProject: false,
			CreatedAt: now, UpdatedAt: now,
		},
		[]string{"hosts:read"},
	); err != nil {
		t.Fatalf("seed support: %v", err)
	}

	hash, _ := user.HashPassword("pw")
	u := &user.User{
		ID: uuid.NewString(), Username: "support-user",
		PasswordHash: hash, Role: user.Role("support"),
		CreatedAt: now,
	}
	if err := db.Users().Create(ctx, u); err != nil {
		t.Fatalf("Create support user: %v", err)
	}

	// In-use → 409.
	w := adminReq(t, r, "DELETE", "/api/v1/admin/roles/support", tok, nil)
	if w.Code != http.StatusConflict {
		t.Errorf("DELETE in-use status=%d want 409", w.Code)
	}

	// Reassign user to viewer so support has no references.
	if _, err := db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, u.ID); err != nil {
		t.Fatalf("reassign: %v", err)
	}

	w = adminReq(t, r, "DELETE", "/api/v1/admin/roles/support", tok, nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("DELETE unused status=%d body=%s want 204", w.Code, w.Body.String())
	}
}
