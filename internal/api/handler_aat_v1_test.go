package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

type aatHandlerFixture struct {
	router  *gin.Engine
	db      *storage.DB
	issuer  *TokenIssuer
	verifier *TokenVerifier
	cache   *optoken.Cache
	rbac    *RBAC
	admin   *user.User
	op      *user.User
	viewer  *user.User
	project *storage.Project
}

func aatHandlerSetup(t *testing.T) *aatHandlerFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	issuer, err := NewTokenIssuer("a", "b", 5*time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBACWithVerifier(issuer, db, verifier)

	admin := seedUserForAPITest(t, db, "alice-admin", user.RoleAdmin)
	op := seedUserForAPITest(t, db, "bob-op", user.RoleOperator)
	viewer := seedUserForAPITest(t, db, "carol-viewer", user.RoleViewer)
	project := seedProjectForAPITest(t, db, "p1", admin)
	_ = db.Projects().AddMember(context.Background(), project.ID, op.ID, user.RoleAdmin)
	_ = db.Projects().AddMember(context.Background(), project.ID, viewer.ID, user.RoleViewer)

	h := NewAATHandler(db, verifier)
	r := gin.New()
	RegisterV1AATRoutes(r, h, rbac)

	return &aatHandlerFixture{
		router: r, db: db, issuer: issuer, verifier: verifier, cache: cache, rbac: rbac,
		admin: admin, op: op, viewer: viewer, project: project,
	}
}

func (f *aatHandlerFixture) tokenFor(u *user.User) string {
	tok, _ := f.issuer.IssueAccess(AccessClaims{UserID: u.ID, Username: u.Username, Role: u.Role})
	return tok
}

// ---- Issuance ----

func TestAATHandler_IssueGlobal_AdminOK(t *testing.T) {
	f := aatHandlerSetup(t)
	body := map[string]any{
		"name":        "ci-runner",
		"role":        "viewer",
		"scopes":      []string{"hosts:read"},
		"ttl_seconds": 3600,
	}
	w := probeReqWithPath(f.router, "POST", "/api/v1/aat", f.tokenFor(f.admin), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp issueAATResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.Token, optoken.AATPrefix) {
		t.Errorf("Token %q missing aat_ prefix", resp.Token)
	}
	if resp.TokenID == "" || resp.ExpiresAt.Before(time.Now()) {
		t.Errorf("missing TokenID/ExpiresAt: %+v", resp)
	}
	// DB row should reflect the issuance.
	a, err := f.db.AuthTokens().GetAAT(context.Background(), resp.TokenID)
	if err != nil {
		t.Fatalf("GetAAT: %v", err)
	}
	if a.UserID != f.admin.ID || a.ProjectID != "" || a.Role != user.RoleViewer {
		t.Errorf("row mismatch: %+v", a)
	}
}

func TestAATHandler_IssueGlobal_NonAdminDenied(t *testing.T) {
	f := aatHandlerSetup(t)
	body := map[string]any{
		"name":        "ci",
		"role":        "viewer",
		"scopes":      []string{"hosts:read"},
		"ttl_seconds": 3600,
	}
	w := probeReqWithPath(f.router, "POST", "/api/v1/aat", f.tokenFor(f.op), body)
	if w.Code != http.StatusForbidden {
		t.Errorf("operator issuing global AAT: status=%d, want 403", w.Code)
	}
}

func TestAATHandler_IssueGlobal_RoleEscalationRejected(t *testing.T) {
	f := aatHandlerSetup(t)
	// Operator can't issue an admin-role AAT.
	op := seedUserForAPITest(t, f.db, "edge-op", user.RoleOperator)
	body := map[string]any{
		"name":        "x",
		"role":        "admin",
		"scopes":      []string{"hosts:read"},
		"ttl_seconds": 3600,
	}
	// Even bypassing the global-admin gate (op isn't admin so it 403s
	// at the gate), this test pins the handler-level guard. Use the
	// project-scoped endpoint so op can reach the handler.
	w := probeReqWithPath(f.router, "POST", "/api/v1/projects/"+f.project.ID+"/aat", f.tokenFor(op), body)
	// op was added as admin on the project in setup; allow this op
	// to reach handler. But we want a fresh op who is project admin
	// but only operator globally.
	_ = f.db.Projects().AddMember(context.Background(), f.project.ID, op.ID, user.RoleAdmin)
	w = probeReqWithPath(f.router, "POST", "/api/v1/projects/"+f.project.ID+"/aat", f.tokenFor(op), body)
	if w.Code != http.StatusForbidden {
		t.Errorf("operator issuing admin AAT: status=%d body=%s; want 403", w.Code, w.Body.String())
	}
}

func TestAATHandler_IssueProject_AdminOK(t *testing.T) {
	f := aatHandlerSetup(t)
	body := map[string]any{
		"name":        "agent-bot",
		"role":        "operator",
		"scopes":      []string{"hosts:read", "hosts:exec"},
		"ttl_seconds": 600,
	}
	w := probeReqWithPath(f.router, "POST", "/api/v1/projects/"+f.project.ID+"/aat", f.tokenFor(f.op), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp issueAATResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	a, _ := f.db.AuthTokens().GetAAT(context.Background(), resp.TokenID)
	if a.ProjectID != f.project.ID {
		t.Errorf("project binding missing: %+v", a)
	}
}

func TestAATHandler_IssueProject_ViewerDenied(t *testing.T) {
	f := aatHandlerSetup(t)
	body := map[string]any{
		"name":        "bad",
		"role":        "viewer",
		"scopes":      []string{"hosts:read"},
		"ttl_seconds": 600,
	}
	w := probeReqWithPath(f.router, "POST", "/api/v1/projects/"+f.project.ID+"/aat", f.tokenFor(f.viewer), body)
	if w.Code != http.StatusForbidden {
		t.Errorf("viewer issuing project AAT: status=%d, want 403", w.Code)
	}
}

func TestAATHandler_IssueRequiresName(t *testing.T) {
	f := aatHandlerSetup(t)
	body := map[string]any{
		"role":   "viewer",
		"scopes": []string{"hosts:read"},
	}
	w := probeReqWithPath(f.router, "POST", "/api/v1/aat", f.tokenFor(f.admin), body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing name: status=%d, want 400", w.Code)
	}
}

func TestAATHandler_IssueScopeEscalationRejected(t *testing.T) {
	f := aatHandlerSetup(t)
	// Viewer-level human cannot grant write scopes.
	body := map[string]any{
		"name":        "leak",
		"role":        "viewer",
		"scopes":      []string{"hosts:exec"},
		"ttl_seconds": 600,
	}
	// Viewer reaches /aat (global) only as a regression — they get 403
	// at the gate. Re-check at the project endpoint they can hit.
	// Promote viewer to project admin first so the gate lets them through.
	_ = f.db.Projects().AddMember(context.Background(), f.project.ID, f.viewer.ID, user.RoleAdmin)
	w := probeReqWithPath(f.router, "POST", "/api/v1/projects/"+f.project.ID+"/aat", f.tokenFor(f.viewer), body)
	if w.Code != http.StatusForbidden {
		t.Errorf("viewer issuing exec scope: status=%d body=%s; want 403", w.Code, w.Body.String())
	}
}

// ---- Get / List / Revoke ----

func mintTestAAT(t *testing.T, f *aatHandlerFixture, creator *user.User, opt func(*storage.AAT)) *storage.AAT {
	t.Helper()
	id, _, hash, _, err := optoken.Generate(optoken.AATPrefix)
	if err != nil {
		t.Fatal(err)
	}
	a := &storage.AAT{
		TokenID:    id,
		SecretHash: hash,
		UserID:     creator.ID,
		Name:       "test-" + id,
		Role:       user.RoleViewer,
		Scopes:     []string{optoken.ScopeHostsRead},
		CreatedAt:  time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	}
	if opt != nil {
		opt(a)
	}
	if err := f.db.AuthTokens().CreateAAT(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAATHandler_GetByCreator(t *testing.T) {
	f := aatHandlerSetup(t)
	a := mintTestAAT(t, f, f.op, nil)
	w := probeReqWithPath(f.router, "GET", "/api/v1/aat/"+a.TokenID, f.tokenFor(f.op), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret") {
		t.Errorf("response leaked secret: %s", w.Body.String())
	}
}

func TestAATHandler_GetByOtherDenied(t *testing.T) {
	f := aatHandlerSetup(t)
	a := mintTestAAT(t, f, f.op, nil)
	// viewer (different user, not admin) attempts to read op's AAT
	w := probeReqWithPath(f.router, "GET", "/api/v1/aat/"+a.TokenID, f.tokenFor(f.viewer), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("viewer reading other's AAT: status=%d, want 404 (not 403, to avoid existence leak)", w.Code)
	}
}

func TestAATHandler_GetByAdminAlwaysAllowed(t *testing.T) {
	f := aatHandlerSetup(t)
	a := mintTestAAT(t, f, f.op, nil)
	w := probeReqWithPath(f.router, "GET", "/api/v1/aat/"+a.TokenID, f.tokenFor(f.admin), nil)
	if w.Code != http.StatusOK {
		t.Errorf("admin reading op's AAT: status=%d", w.Code)
	}
}

func TestAATHandler_ListMine(t *testing.T) {
	f := aatHandlerSetup(t)
	mintTestAAT(t, f, f.op, nil)
	mintTestAAT(t, f, f.op, nil)
	mintTestAAT(t, f, f.viewer, nil) // shouldn't show up

	w := probeReqWithPath(f.router, "GET", "/api/v1/aat", f.tokenFor(f.op), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var resp struct {
		Tokens []aatListItem `json:"tokens"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Tokens) != 2 {
		t.Errorf("len(tokens)=%d, want 2 (op's own only)", len(resp.Tokens))
	}
	for _, tok := range resp.Tokens {
		if tok.UserID != f.op.ID {
			t.Errorf("leaked AAT from %q (not creator)", tok.UserID)
		}
	}
}

func TestAATHandler_RevokeByCreator(t *testing.T) {
	f := aatHandlerSetup(t)
	id, _, hash, plaintext, _ := optoken.Generate(optoken.AATPrefix)
	a := &storage.AAT{
		TokenID: id, SecretHash: hash, UserID: f.op.ID,
		Name: "rev", Role: user.RoleViewer, Scopes: []string{optoken.ScopeHostsRead},
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := f.db.AuthTokens().CreateAAT(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	// Prime cache by verifying first.
	if _, reason, _ := f.verifier.Verify(context.Background(), plaintext); reason != "success" {
		t.Fatal("priming verify failed")
	}

	w := probeReqWithPath(f.router, "DELETE", "/api/v1/aat/"+a.TokenID, f.tokenFor(f.op), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	// Cache must be invalidated synchronously: a re-verify hits DB,
	// returns "revoked".
	_, reason, _ := f.verifier.Verify(context.Background(), plaintext)
	if reason != "revoked" {
		t.Errorf("post-revoke verify reason=%q, want revoked (cache invalidate missing?)", reason)
	}
}

func TestAATHandler_RevokeByOther_404(t *testing.T) {
	f := aatHandlerSetup(t)
	a := mintTestAAT(t, f, f.op, nil)
	w := probeReqWithPath(f.router, "DELETE", "/api/v1/aat/"+a.TokenID, f.tokenFor(f.viewer), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("non-creator revoke: status=%d, want 404", w.Code)
	}
}

func TestAATHandler_AAT_PrincipalCannotMint(t *testing.T) {
	f := aatHandlerSetup(t)
	// Mint an admin-role AAT first, then try to use IT to mint another.
	id, _, hash, plaintext, _ := optoken.Generate(optoken.AATPrefix)
	a := &storage.AAT{
		TokenID: id, SecretHash: hash, UserID: f.admin.ID,
		Name: "self", Role: user.RoleAdmin, Scopes: optoken.AllScopes(),
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := f.db.AuthTokens().CreateAAT(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"name":        "child",
		"role":        "viewer",
		"scopes":      []string{"hosts:read"},
		"ttl_seconds": 600,
	}
	w := probeReqWithPath(f.router, "POST", "/api/v1/aat", plaintext, body)
	if w.Code != http.StatusForbidden {
		t.Errorf("AAT minting another AAT: status=%d, want 403 (humans only)", w.Code)
	}
}
