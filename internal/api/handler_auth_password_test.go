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

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// passwordTestSetup mounts ChangePassword behind RequireAuth so the test
// exercises the full middleware chain a real client hits. Returns the
// router, DB, issuer, and the seeded user + password so tests can log
// that user in first.
func passwordTestSetup(t *testing.T) (*gin.Engine, *storage.DB, *TokenIssuer, *user.User, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	issuer, _ := NewTokenIssuer("a", "b", 15*time.Minute, 24*time.Hour)
	h := NewAuthHandler(db, issuer, "unused")
	rbac := NewRBAC(issuer)

	r := gin.New()
	g := r.Group("/api/v1/auth")
	g.Use(rbac.RequireAuth())
	g.PATCH("/password", h.ChangePassword)

	// Seed a user with a known password.
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
	return r, db, issuer, u, plain
}

func passwordReq(r http.Handler, token string, oldP, newP string) *httptest.ResponseRecorder {
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

func issueAccessFor(t *testing.T, issuer *TokenIssuer, u *user.User) string {
	t.Helper()
	tok, err := issuer.IssueAccess(AccessClaims{
		UserID: u.ID, Username: u.Username, Role: u.Role,
	})
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	return tok
}

func TestChangePassword_RequiresAuth(t *testing.T) {
	r, _, _, _, _ := passwordTestSetup(t)

	w := passwordReq(r, "", "old", "new-correct")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d; want 401", w.Code)
	}
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	r, _, issuer, u, _ := passwordTestSetup(t)
	tok := issueAccessFor(t, issuer, u)

	w := passwordReq(r, tok, "not-the-old-one", "new-secure-one")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-old status=%d; want 401", w.Code)
	}
}

func TestChangePassword_EmptyNewPassword(t *testing.T) {
	r, _, issuer, u, old := passwordTestSetup(t)
	tok := issueAccessFor(t, issuer, u)

	// Empty new_password must be rejected — an empty-password account
	// is an accident waiting to happen, not a feature.
	w := passwordReq(r, tok, old, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty-new status=%d; want 400", w.Code)
	}
}

func TestChangePassword_SuccessRotatesHashAndRevokesRefreshTokens(t *testing.T) {
	r, db, issuer, u, old := passwordTestSetup(t)
	ctx := context.Background()

	// Seed two refresh tokens for the user so the revoke-all behaviour
	// is observable.
	for _, id := range []string{"rt-1", "rt-2"} {
		_ = db.RefreshTokens().Create(ctx, &storage.RefreshToken{
			ID: id, UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour).UTC(),
		})
	}

	tok := issueAccessFor(t, issuer, u)
	w := passwordReq(r, tok, old, "new-secure-pw")
	if w.Code != http.StatusNoContent {
		t.Fatalf("success status=%d body=%s; want 204", w.Code, w.Body.String())
	}

	// The stored hash now verifies against the new password, not the old.
	got, _ := db.Users().GetByID(ctx, u.ID)
	if user.VerifyPassword(got.PasswordHash, old) {
		t.Fatal("old password still verifies after change")
	}
	if !user.VerifyPassword(got.PasswordHash, "new-secure-pw") {
		t.Fatal("new password does not verify after change")
	}

	// Both refresh tokens are now revoked.
	for _, id := range []string{"rt-1", "rt-2"} {
		rt, err := db.RefreshTokens().Get(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if rt.RevokedAt == nil {
			t.Errorf("refresh token %s still live after password change", id)
		}
	}
}
