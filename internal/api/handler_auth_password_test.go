package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// passwordTestSetup mounts ChangePassword behind RequireAuth so the
// test exercises the full middleware chain. The router accepts the
// user's session_token as bearer (post-Phase-2 auth shape). Returns
// the router, DB, verifier, the seeded user, and their plaintext
// password.
func passwordTestSetup(t *testing.T) (*gin.Engine, *storage.DB, *TokenVerifier, *user.User, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	issuer, _ := NewTokenIssuer("a", "b", 15*time.Minute, 24*time.Hour)
	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	h := NewAuthHandler(db, verifier, "unused")
	rbac := NewRBACWithVerifier(issuer, db, verifier)

	r := gin.New()
	g := r.Group("/api/v1/auth")
	g.Use(rbac.RequireAuth())
	g.PATCH("/password", h.ChangePassword)

	plain := "correct horse battery staple"
	hash, _ := user.HashPassword(plain)
	u := &user.User{
		ID: uuid.NewString(), Username: "alice",
		PasswordHash: hash, Role: user.RoleOperator,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return r, db, verifier, u, plain
}

// mintSessionFor seeds an auth_tokens row and returns the plaintext
// session token, mimicking what Login would produce.
func mintSessionFor(t *testing.T, db *storage.DB, u *user.User) string {
	t.Helper()
	id, _, hash, plaintext, err := optoken.Generate(optoken.UserSessionPrefix)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s := &storage.UserSession{
		TokenID: id, SecretHash: hash, UserID: u.ID,
		CreatedAt: now, ExpiresAt: now.Add(SessionHardTTL),
		IdleExpiresAt: now.Add(SessionIdleWindow),
	}
	if err := db.AuthTokens().CreateSession(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return plaintext
}

func passwordReq(r http.Handler, token, oldP, newP string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{
		"old_password": oldP,
		"new_password": newP,
	})
	req := httptest.NewRequest("PATCH", "/api/v1/auth/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestChangePassword_RequiresAuth(t *testing.T) {
	r, _, _, _, _ := passwordTestSetup(t)
	w := passwordReq(r, "", "old", "new-correct")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d; want 401", w.Code)
	}
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	r, db, _, u, _ := passwordTestSetup(t)
	tok := mintSessionFor(t, db, u)
	w := passwordReq(r, tok, "not-the-old-one", "new-secure-one")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-old status=%d; want 401", w.Code)
	}
}

func TestChangePassword_EmptyNewPassword(t *testing.T) {
	r, db, _, u, old := passwordTestSetup(t)
	tok := mintSessionFor(t, db, u)
	w := passwordReq(r, tok, old, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty-new status=%d; want 400", w.Code)
	}
}

func TestChangePassword_SuccessRotatesAndCascadesSessions(t *testing.T) {
	r, db, _, u, old := passwordTestSetup(t)
	ctx := context.Background()

	// Mint THREE sessions: one is the caller's, two are "other devices"
	// that should be revoked on password change.
	caller := mintSessionFor(t, db, u)
	other1 := mintSessionFor(t, db, u)
	other2 := mintSessionFor(t, db, u)

	w := passwordReq(r, caller, old, "new-secure-pw")
	if w.Code != http.StatusNoContent {
		t.Fatalf("success status=%d body=%s; want 204", w.Code, w.Body.String())
	}

	// Hash rotated.
	got, _ := db.Users().GetByID(ctx, u.ID)
	if user.VerifyPassword(got.PasswordHash, old) {
		t.Fatal("old password still verifies")
	}
	if !user.VerifyPassword(got.PasswordHash, "new-secure-pw") {
		t.Fatal("new password does not verify")
	}

	// Caller's own session preserved (revoking it would kick the user
	// out of the very page they used to change password).
	active, _ := db.AuthTokens().ListSessionsForUser(ctx, u.ID)
	callerStillAlive := false
	otherCount := 0
	for _, s := range active {
		callerID, _, _ := optoken.Parse(caller, optoken.UserSessionPrefix)
		if s.TokenID == callerID {
			callerStillAlive = true
		}
		otherCount++ // count any active row
		_ = other1
		_ = other2
	}
	if !callerStillAlive {
		t.Error("caller's own session revoked — user would be kicked out")
	}
	// Only one active row should remain (the caller's).
	if otherCount != 1 {
		t.Errorf("active sessions = %d, want 1 (caller only)", otherCount)
	}
}
