package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// patHandlerSetup wires the Account-PAT handler behind RequireAuth so
// tests exercise the full middleware chain — bearer-session in,
// principal on context, handler downstream.
func patHandlerSetup(t *testing.T) (*gin.Engine, *storage.DB, *api.TokenVerifier) {
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
	h := api.NewAccountPATHandler(db, verifier)

	r := gin.New()
	api.RegisterV1AccountPATRoutes(r, h, rbac)
	return r, db, verifier
}

// patSeedUser inserts a user and returns (user, plaintext-session-token).
func patSeedUser(t *testing.T, db *storage.DB, username string, role user.Role) (*user.User, string) {
	t.Helper()
	hash, _ := user.HashPassword("pw-" + username)
	u := &user.User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}
	if err := db.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("Create user: %v", err)
	}

	id, _, secretHash, plaintext, err := optoken.Generate(optoken.UserSessionPrefix)
	if err != nil {
		t.Fatalf("Generate session: %v", err)
	}
	now := time.Now().UTC()
	s := &storage.UserSession{
		TokenID:       id,
		SecretHash:    secretHash,
		UserID:        u.ID,
		CreatedAt:     now,
		ExpiresAt:     now.Add(30 * 24 * time.Hour),
		IdleExpiresAt: now.Add(time.Hour),
	}
	if err := db.AuthTokens().CreateSession(context.Background(), s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return u, plaintext
}

func patReq(t *testing.T, r *gin.Engine, method, path, bearer string, body any) *httptest.ResponseRecorder {
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

func TestAccountPAT_RequiresAuth(t *testing.T) {
	t.Parallel()
	r, _, _ := patHandlerSetup(t)
	for _, p := range []struct{ method, path string }{
		{"POST", "/api/v1/account/pat"},
		{"GET", "/api/v1/account/pat"},
		{"GET", "/api/v1/account/pat/pat_x"},
		{"DELETE", "/api/v1/account/pat/pat_x"},
	} {
		w := patReq(t, r, p.method, p.path, "", nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without bearer = %d, want 401", p.method, p.path, w.Code)
		}
	}
}

func TestAccountPAT_Issue_Admin_AllScopes(t *testing.T) {
	t.Parallel()
	r, db, _ := patHandlerSetup(t)
	_, sess := patSeedUser(t, db, "admin1", user.RoleAdmin)

	w := patReq(t, r, "POST", "/api/v1/account/pat", sess, map[string]any{
		"name":        "ci-bot",
		"description": "for the CI runner",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		TokenID   string   `json:"token_id"`
		Token     string   `json:"token"`
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		ExpiresAt string   `json:"expires_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.HasPrefix(resp.Token, "pat_") {
		t.Errorf("plaintext token does not have pat_ prefix: %q", resp.Token)
	}
	if resp.Name != "ci-bot" {
		t.Errorf("name = %q", resp.Name)
	}
	if len(resp.Scopes) == 0 {
		t.Error("default scopes empty for admin issuer")
	}
	if !contains(resp.Scopes, optoken.ScopeHostsExec) {
		t.Errorf("admin default scopes missing hosts:exec: %v", resp.Scopes)
	}
}

func TestAccountPAT_Issue_Viewer_RejectsWriteScope(t *testing.T) {
	t.Parallel()
	r, db, _ := patHandlerSetup(t)
	_, sess := patSeedUser(t, db, "viewer1", user.RoleViewer)

	w := patReq(t, r, "POST", "/api/v1/account/pat", sess, map[string]any{
		"name":   "read-bot",
		"scopes": []string{optoken.ScopeHostsExec}, // write scope viewer doesn't hold
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (viewer escalating to write scope)", w.Code)
	}
}

func TestAccountPAT_Issue_Viewer_DefaultsToReadOnly(t *testing.T) {
	t.Parallel()
	r, db, _ := patHandlerSetup(t)
	_, sess := patSeedUser(t, db, "viewer2", user.RoleViewer)

	w := patReq(t, r, "POST", "/api/v1/account/pat", sess, map[string]any{
		"name": "read-only-bot",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Scopes []string `json:"scopes"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if contains(resp.Scopes, optoken.ScopeHostsExec) || contains(resp.Scopes, optoken.ScopeFilesWrite) {
		t.Errorf("viewer default scopes included write: %v", resp.Scopes)
	}
	if !contains(resp.Scopes, optoken.ScopeHostsRead) {
		t.Errorf("viewer default scopes missing hosts:read: %v", resp.Scopes)
	}
}

func TestAccountPAT_Issue_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	r, db, _ := patHandlerSetup(t)
	_, sess := patSeedUser(t, db, "admin2", user.RoleAdmin)

	w := patReq(t, r, "POST", "/api/v1/account/pat", sess, map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (missing name)", w.Code)
	}
}

func TestAccountPAT_List_OnlyOwnTokens(t *testing.T) {
	t.Parallel()
	r, db, _ := patHandlerSetup(t)
	_, aliceSess := patSeedUser(t, db, "alice", user.RoleOperator)
	_, bobSess := patSeedUser(t, db, "bob", user.RoleOperator)

	for i := 0; i < 2; i++ {
		w := patReq(t, r, "POST", "/api/v1/account/pat", aliceSess, map[string]any{
			"name": "alice-token",
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("alice issue %d: %d", i, w.Code)
		}
	}
	w := patReq(t, r, "POST", "/api/v1/account/pat", bobSess, map[string]any{"name": "bob-token"})
	if w.Code != http.StatusCreated {
		t.Fatalf("bob issue: %d", w.Code)
	}

	w = patReq(t, r, "GET", "/api/v1/account/pat", aliceSess, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
	var resp struct {
		Tokens []struct {
			TokenID string `json:"token_id"`
			Name    string `json:"name"`
		} `json:"tokens"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Tokens) != 2 {
		t.Errorf("alice list = %d tokens, want 2 (got %+v)", len(resp.Tokens), resp.Tokens)
	}
	for _, tk := range resp.Tokens {
		if tk.Name != "alice-token" {
			t.Errorf("alice list leaked %q", tk.Name)
		}
	}
}

func TestAccountPermissions_ReturnsCallerRolePermissions(t *testing.T) {
	t.Parallel()
	r, db, _ := patHandlerSetup(t)
	_, opSess := patSeedUser(t, db, "op-perm-list", user.RoleOperator)
	_, viewerSess := patSeedUser(t, db, "viewer-perm-list", user.RoleViewer)

	for _, tc := range []struct {
		name, sess string
		mustHave   []string
		mustNot    []string
	}{
		{
			name:     "operator",
			sess:     opSess,
			mustHave: []string{"hosts:exec", "files:write"},
			mustNot:  []string{"admin:users"},
		},
		{
			name:     "viewer",
			sess:     viewerSess,
			mustHave: []string{"hosts:read"},
			mustNot:  []string{"hosts:exec", "files:write", "admin:users"},
		},
	} {
		w := patReq(t, r, "GET", "/api/v1/account/permissions", tc.sess, nil)
		if w.Code != http.StatusOK {
			t.Errorf("%s status=%d body=%s", tc.name, w.Code, w.Body.String())
			continue
		}
		var resp struct {
			Permissions []string `json:"permissions"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		for _, must := range tc.mustHave {
			if !contains(resp.Permissions, must) {
				t.Errorf("%s missing %q in %v", tc.name, must, resp.Permissions)
			}
		}
		for _, no := range tc.mustNot {
			if contains(resp.Permissions, no) {
				t.Errorf("%s unexpectedly has %q in %v", tc.name, no, resp.Permissions)
			}
		}
	}
}

func TestAccountPAT_RevokeIdempotent(t *testing.T) {
	t.Parallel()
	r, db, _ := patHandlerSetup(t)
	_, sess := patSeedUser(t, db, "admin3", user.RoleAdmin)

	w := patReq(t, r, "POST", "/api/v1/account/pat", sess, map[string]any{"name": "to-kill"})
	if w.Code != http.StatusCreated {
		t.Fatalf("issue: %d", w.Code)
	}
	var issued struct {
		TokenID string `json:"token_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &issued)

	w = patReq(t, r, "DELETE", "/api/v1/account/pat/"+issued.TokenID, sess, nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("first revoke = %d", w.Code)
	}
	// Second revoke is idempotent.
	w = patReq(t, r, "DELETE", "/api/v1/account/pat/"+issued.TokenID, sess, nil)
	if w.Code != http.StatusNoContent {
		t.Errorf("second revoke = %d, want 204 (idempotent)", w.Code)
	}
}

func TestAccountPAT_PlaintextWorksAsBearer(t *testing.T) {
	t.Parallel()
	r, db, verifier := patHandlerSetup(t)
	_, sess := patSeedUser(t, db, "admin4", user.RoleAdmin)

	w := patReq(t, r, "POST", "/api/v1/account/pat", sess, map[string]any{"name": "as-bearer"})
	if w.Code != http.StatusCreated {
		t.Fatalf("issue: %d", w.Code)
	}
	var issued struct {
		TokenID string `json:"token_id"`
		Token   string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &issued)

	// Use the freshly-issued PAT to call the LIST endpoint — proves
	// the verifier authenticates pat_ tokens end-to-end.
	w = patReq(t, r, "GET", "/api/v1/account/pat", issued.Token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list with PAT bearer = %d, body=%s", w.Code, w.Body.String())
	}
	_ = verifier
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
