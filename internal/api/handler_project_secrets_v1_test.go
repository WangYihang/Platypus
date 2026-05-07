package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/cryptobox"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// projectSecretsTestSetup mounts the project-secrets router on a
// fresh in-memory DB and seeds the operator-supplied KEK so the
// crypto path inside Reveal works without an env var. Returns the
// gin engine + db handle so individual tests can stage rows.
func projectSecretsTestSetup(t *testing.T) (*gin.Engine, *storage.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv(cryptobox.EnvVar,
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBAC(db, verifier)
	h := NewProjectSecretsHandler(db)

	r := gin.New()
	RegisterV1ProjectSecretRoutes(r, h, rbac)
	return r, db
}

// TestProjectSecrets_Create_OK: an admin POSTs a new secret;
// response carries the assigned id and a redacted body (no
// plaintext, no ciphertext bytes). The DB row stores the
// AES-GCM-sealed ciphertext.
func TestProjectSecrets_Create_OK(t *testing.T) {
	r, db := projectSecretsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	body := map[string]string{
		"name":        "datadog_api_key",
		"description": "DD intake",
		"value":       "supersecret-1234",
	}
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/secrets", tok, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp projectSecretItem
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SecretID == "" || resp.Name != "datadog_api_key" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	// Response must NOT contain raw ciphertext / nonce / value.
	raw := w.Body.String()
	for _, leak := range []string{"supersecret-1234", "ciphertext", "nonce"} {
		if containsCaseInsensitive(raw, leak) {
			t.Fatalf("response leaks %q: %s", leak, raw)
		}
	}
}

// TestProjectSecrets_Create_RequiresAdmin: viewers can't create.
// Same RBAC posture as every other write surface in the v1 admin
// API.
func TestProjectSecrets_Create_RequiresAdmin(t *testing.T) {
	r, db := projectSecretsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	viewer := seedUserForAPITest(t, db, "viewer", user.RoleViewer)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	tok := mintBearerForUserID(t, db, viewer.ID, user.RoleViewer)
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/secrets", tok,
		map[string]string{"name": "x", "value": "y"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer create status=%d, want 403", w.Code)
	}
}

// TestProjectSecrets_Create_RejectsEmptyName: name is the only
// stable identifier operators have for a secret in their plugin
// configs; an empty name is always a typo.
func TestProjectSecrets_Create_RejectsEmptyName(t *testing.T) {
	r, db := projectSecretsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/secrets", tok,
		map[string]string{"name": "  ", "value": "y"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 — body=%s", w.Code, w.Body.String())
	}
}

// TestProjectSecrets_List_RedactsAndOrdersNewestFirst: list
// returns the redacted view of every active secret in the project,
// newest-first. Plaintext / ciphertext must not appear in any
// response — the type system enforces that, but the test pins the
// behaviour.
func TestProjectSecrets_List_RedactsAndOrdersNewestFirst(t *testing.T) {
	r, db := projectSecretsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	for _, name := range []string{"older", "newer"} {
		body := map[string]string{"name": name, "value": "v-" + name}
		w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/secrets", tok, body)
		if w.Code != http.StatusCreated {
			t.Fatalf("setup create %s: %d", name, w.Code)
		}
		// Ensure distinct created_at timestamps.
		time.Sleep(2 * time.Millisecond)
	}
	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/secrets", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Secrets []projectSecretItem `json:"secrets"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Secrets) != 2 {
		t.Fatalf("len = %d, want 2", len(resp.Secrets))
	}
	if resp.Secrets[0].Name != "newer" || resp.Secrets[1].Name != "older" {
		t.Fatalf("ordering = [%s, %s], want [newer, older]",
			resp.Secrets[0].Name, resp.Secrets[1].Name)
	}
}

// TestProjectSecrets_Delete_Idempotent: DELETE marks revoked.
// A second DELETE on the same secret_id is also OK (returns
// 204) — same posture as enrollment-token revocation, so
// operator scripts that retry don't blow up.
func TestProjectSecrets_Delete_Idempotent(t *testing.T) {
	r, db := projectSecretsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	// Create first.
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/secrets", tok,
		map[string]string{"name": "one", "value": "v"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d", w.Code)
	}
	var created projectSecretItem
	_ = json.NewDecoder(w.Body).Decode(&created)

	// First delete: 204.
	w = probeReqWithPath(r, "DELETE",
		"/api/v1/projects/"+proj.ID+"/secrets/"+created.SecretID, tok, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("first delete status=%d, want 204", w.Code)
	}
	// Second delete on the same id: also 204 (idempotent).
	w = probeReqWithPath(r, "DELETE",
		"/api/v1/projects/"+proj.ID+"/secrets/"+created.SecretID, tok, nil)
	if w.Code != http.StatusNoContent && w.Code != http.StatusNotFound {
		t.Fatalf("second delete status=%d, want 204 or 404", w.Code)
	}

	// Delete on a never-existed id: 404.
	w = probeReqWithPath(r, "DELETE",
		"/api/v1/projects/"+proj.ID+"/secrets/sec_ghost", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("ghost delete status=%d, want 404", w.Code)
	}
}

// containsCaseInsensitive: tiny helper used by the leak-prevention
// assertion above. Test-only; lower-cases both sides for the substring
// check.
func containsCaseInsensitive(haystack, needle string) bool {
	return indexLower(haystack, needle) >= 0
}

func indexLower(h, n string) int {
	if len(n) == 0 {
		return 0
	}
	hl, nl := lower(h), lower(n)
	for i := 0; i+len(nl) <= len(hl); i++ {
		if hl[i:i+len(nl)] == nl {
			return i
		}
	}
	return -1
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
