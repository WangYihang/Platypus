package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/pki"
	"github.com/WangYihang/Platypus/internal/storage"
)

// TestStartupFlow is a cross-layer regression test for the four fixes
// that together make `docker compose down -v && docker compose up`
// work out of the box: KEK auto-generation, seed of the system user
// and default project, mesh self-issue reason="admin", and the Count
// filter that keeps the seeded system user from flipping the bootstrap
// "already initialised" check.
//
// If any one of those pieces regresses — a renamed constant, a
// tightened CHECK constraint, a dropped fallback — this test fails
// loudly at the seam rather than each layer quietly still passing its
// own unit tests.
func TestStartupFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Force readKEK down the file-fallback branch (the dev path the
	// server main wires up for docker) and give it a fresh directory
	// so we can assert FS side-effects without collision.
	kekDir := t.TempDir()
	kekPath := filepath.Join(kekDir, "ca.kek")
	t.Setenv(pki.KEKEnvVar, "")
	prev := pki.KEKPath
	pki.KEKPath = kekPath
	t.Cleanup(func() { pki.KEKPath = prev })

	ctx := context.Background()

	// Mount the full auth router (auth endpoints + admin-gated
	// /api/v1/users) the same way cmd/platypus-server/main.go does,
	// so the bootstrap → login → users-list path we exercise is the
	// production surface, not a test-only subset.
	const bootstrapSecretValue = "startup-integration-secret"
	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	engine := CreateRESTfulAPIServer()
	RegisterV1AuthRoutes(
		engine,
		NewAuthHandler(db, verifier, bootstrapSecretValue),
		NewUsersHandler(db),
		NewRBAC(db, verifier),
	)

	pkiSvc := pki.New(db)

	// Captured in FreshInstall, re-read in SecondBootIdempotent to
	// prove idempotency: the CA cert PEM and KEK bytes must be
	// byte-identical across the two subtests.
	var firstCACert string
	var firstKEKBytes []byte

	t.Run("FreshInstall", func(t *testing.T) {
		// 1. Seed (fix #2: project_ca FKs now resolve on first boot).
		sysID, err := storage.EnsureSystemUser(ctx, db)
		if err != nil {
			t.Fatalf("EnsureSystemUser: %v", err)
		}
		if sysID != storage.SystemUserID {
			t.Fatalf("sysID = %q; want %q", sysID, storage.SystemUserID)
		}
		if _, err := storage.EnsureDefaultProject(ctx, db, sysID); err != nil {
			t.Fatalf("EnsureDefaultProject: %v", err)
		}

		// 2. EnsureCA (fix #1: KEK auto-generated at kekPath; fix #2:
		// created_by_user + project_id FKs satisfied).
		ca, err := pkiSvc.EnsureCA(ctx, storage.DefaultProjectID, storage.SystemUserID)
		if err != nil {
			t.Fatalf("EnsureCA: %v", err)
		}
		if ca.CertPEM == "" {
			t.Fatalf("EnsureCA returned empty CertPEM")
		}
		firstCACert = ca.CertPEM

		// 3. Self-issue a leaf with reason="admin" — the exact call
		// tryStartServerMesh makes (fix #3; without it the CHECK
		// constraint rejects the insert).
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("ed25519.GenerateKey: %v", err)
		}
		certPEM, caPEM, err := pkiSvc.IssueForAgent(ctx, storage.DefaultProjectID, "server", pub, "admin")
		if err != nil {
			t.Fatalf("IssueForAgent(reason=admin): %v", err)
		}
		if certPEM == "" || caPEM == "" {
			t.Fatalf("IssueForAgent returned empty PEMs: cert=%q ca=%q", certPEM, caPEM)
		}
		var reasonInDB string
		err = db.QueryRowContext(ctx,
			`SELECT issued_reason FROM issued_certs WHERE project_id = ? AND agent_id = ?`,
			storage.DefaultProjectID, "server",
		).Scan(&reasonInDB)
		if err != nil {
			t.Fatalf("select issued_reason: %v", err)
		}
		if reasonInDB != "admin" {
			t.Fatalf("issued_reason = %q; want %q", reasonInDB, "admin")
		}

		// 4. Negative: a reason outside the documented enum must be
		// rejected by the CHECK constraint. Pins the schema contract
		// so a future migration that widens the enum doesn't silently
		// let bogus values through.
		pubBogus, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("ed25519.GenerateKey (bogus): %v", err)
		}
		if _, _, err := pkiSvc.IssueForAgent(ctx, storage.DefaultProjectID, "server-bogus", pubBogus, "bogus-reason"); err == nil {
			t.Fatalf("IssueForAgent(reason=bogus) succeeded; want CHECK constraint error")
		}

		// 5. Bootstrap (fix #4: Count() hides the seeded system user,
		// so a fresh install doesn't 409).
		w := probeReqWithPath(engine, "POST", "/api/v1/auth/bootstrap", "", map[string]string{
			"secret":   bootstrapSecretValue,
			"username": "root",
			"password": "correct horse battery staple",
		})
		if w.Code != http.StatusOK {
			t.Fatalf("bootstrap: status=%d body=%s", w.Code, w.Body.String())
		}
		var pair loginBody
		if err := json.NewDecoder(w.Body).Decode(&pair); err != nil {
			t.Fatalf("decode bootstrap body: %v", err)
		}
		if pair.SessionToken == "" {
			t.Fatalf("bootstrap returned empty session_token")
		}

		// 6. Login with the freshly-bootstrapped admin.
		w = probeReqWithPath(engine, "POST", "/api/v1/auth/login", "", map[string]string{
			"username": "root",
			"password": "correct horse battery staple",
		})
		if w.Code != http.StatusOK {
			t.Fatalf("login: status=%d body=%s", w.Code, w.Body.String())
		}
		var loginPair loginBody
		if err := json.NewDecoder(w.Body).Decode(&loginPair); err != nil {
			t.Fatalf("decode login body: %v", err)
		}

		// 7. List users — must return only the human admin.
		w = probeReqWithPath(engine, "GET", "/api/v1/users", loginPair.SessionToken, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /users: status=%d body=%s", w.Code, w.Body.String())
		}
		var listResp struct {
			Users []struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"users"`
		}
		if err := json.NewDecoder(w.Body).Decode(&listResp); err != nil {
			t.Fatalf("decode users body: %v", err)
		}
		if len(listResp.Users) != 1 {
			t.Fatalf("users length = %d; want 1 (system user must be hidden); body=%+v", len(listResp.Users), listResp.Users)
		}
		if listResp.Users[0].Username != "root" {
			t.Fatalf("users[0].username = %q; want %q", listResp.Users[0].Username, "root")
		}

		// 8. Targeted lookup by id still returns the system user —
		// the filter is scoped to List/Count, not the row itself.
		sys, err := db.Users().GetByID(ctx, storage.SystemUserID)
		if err != nil {
			t.Fatalf("GetByID(system): %v", err)
		}
		if sys.Username != storage.SystemUserID {
			t.Fatalf("system user username = %q; want %q", sys.Username, storage.SystemUserID)
		}

		// 9. KEK file (fix #1: 0600 perms, 32 bytes of hex).
		info, err := os.Stat(kekPath)
		if err != nil {
			t.Fatalf("stat kek: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("kek file perms = %o; want 0600", perm)
		}
		raw, err := os.ReadFile(kekPath)
		if err != nil {
			t.Fatalf("read kek: %v", err)
		}
		kekBytes, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatalf("kek contents not hex: %v", err)
		}
		if len(kekBytes) != 32 {
			t.Fatalf("kek decoded length = %d; want 32", len(kekBytes))
		}
		firstKEKBytes = kekBytes
	})

	t.Run("SecondBootIdempotent", func(t *testing.T) {
		if firstCACert == "" || len(firstKEKBytes) == 0 {
			t.Fatalf("FreshInstall must run first")
		}

		// Re-seeding is a no-op: no error, same ids.
		sysID, err := storage.EnsureSystemUser(ctx, db)
		if err != nil {
			t.Fatalf("EnsureSystemUser (second): %v", err)
		}
		if sysID != storage.SystemUserID {
			t.Fatalf("sysID on second boot = %q; want %q", sysID, storage.SystemUserID)
		}
		proj, err := storage.EnsureDefaultProject(ctx, db, sysID)
		if err != nil {
			t.Fatalf("EnsureDefaultProject (second): %v", err)
		}
		if proj.ID != storage.DefaultProjectID {
			t.Fatalf("project id on second boot = %q; want %q", proj.ID, storage.DefaultProjectID)
		}

		// EnsureCA is idempotent — same row, same cert.
		ca, err := pkiSvc.EnsureCA(ctx, storage.DefaultProjectID, storage.SystemUserID)
		if err != nil {
			t.Fatalf("EnsureCA (second): %v", err)
		}
		if ca.CertPEM != firstCACert {
			t.Fatalf("CA cert changed across boots; idempotency broken")
		}

		// KEK file is reused, not regenerated.
		raw, err := os.ReadFile(kekPath)
		if err != nil {
			t.Fatalf("read kek (second): %v", err)
		}
		kekBytes, err := hex.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatalf("kek contents not hex (second): %v", err)
		}
		if string(kekBytes) != string(firstKEKBytes) {
			t.Fatalf("KEK bytes changed across boots; idempotency broken")
		}

		// Bootstrap now returns 409 (admin exists) — this is the
		// assertion that would have failed before fix #4, because
		// the seeded system user alone would have already tripped
		// the "n > 0" guard even on a first real boot.
		w := probeReqWithPath(engine, "POST", "/api/v1/auth/bootstrap", "", map[string]string{
			"secret":   bootstrapSecretValue,
			"username": "root2",
			"password": "another",
		})
		if w.Code != http.StatusConflict {
			t.Fatalf("second bootstrap: status=%d body=%s; want 409", w.Code, w.Body.String())
		}
	})
}
