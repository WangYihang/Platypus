package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

const bootstrapSecret = "bootstrap-secret"

// authTestSetup brings up a router that mounts the full Phase-2 auth
// surface and returns the router + DB so tests can drive the real
// HTTP path and assert against the underlying rows.
func authTestSetup(t *testing.T) (*gin.Engine, *storage.DB) {
	t.Helper()
	// bcrypt at production cost 12 takes ~250ms / hash on 2025 hardware,
	// or ~1.5-2s under -race overhead. The auth tests run several
	// hashes per case (bootstrap, dummy-bcrypt timing flatten on
	// every wrong-password attempt, plus rate-limit isolation that
	// fires 5 wrong-password attempts in a row). At cost 12 the
	// `internal/api` package alone exceeds the Makefile's
	// per-binary timeout. Drop to bcrypt minimum for tests; the
	// resulting hash still verifies, so the test path is end-to-end.
	user.SetPasswordHashCostForTest(t, bcrypt.MinCost)

	gin.SetMode(gin.TestMode)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBAC(db, verifier)

	h := NewAuthHandler(db, verifier, bootstrapSecret)
	r := gin.New()
	pub := r.Group("/api/v1/auth")
	pub.POST("/bootstrap", h.Bootstrap)
	pub.POST("/login", h.Login)
	pub.POST("/refresh", h.Refresh)

	authed := r.Group("/api/v1/auth")
	authed.Use(rbac.RequireAuth())
	authed.POST("/logout", h.Logout)
	authed.GET("/sessions", h.ListSessions)
	return r, db
}

func jsonReq(t *testing.T, r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// loginBody is the parsed shape of /login + /bootstrap responses.
type loginBody struct {
	SessionToken  string    `json:"session_token"`
	TokenID       string    `json:"token_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	IdleExpiresAt time.Time `json:"idle_expires_at"`
	User          struct {
		ID       string    `json:"id"`
		Username string    `json:"username"`
		Role     user.Role `json:"role"`
	} `json:"user"`
}

func TestBootstrap_CreatesFirstAdmin(t *testing.T) {
	r, db := authTestSetup(t)

	w := jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "root",
		"password": "correct horse battery staple",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("Bootstrap status=%d body=%s", w.Code, w.Body.String())
	}
	var got loginBody
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(got.SessionToken, optoken.UserSessionPrefix) {
		t.Errorf("SessionToken %q missing pst_ prefix", got.SessionToken)
	}
	if got.TokenID == "" || got.ExpiresAt.Before(time.Now()) {
		t.Errorf("missing TokenID/ExpiresAt: %+v", got)
	}
	if got.User.Username != "root" || got.User.Role != user.RoleAdmin {
		t.Errorf("user mismatch: %+v", got.User)
	}
	n, _ := db.Users().Count(context.Background())
	if n != 1 {
		t.Errorf("user count = %d; want 1", n)
	}
	// Session row exists.
	s, err := db.AuthTokens().GetSession(context.Background(), got.TokenID)
	if err != nil {
		t.Fatalf("session row missing: %v", err)
	}
	if s.UserID != got.User.ID {
		t.Errorf("session UserID = %q, want %q", s.UserID, got.User.ID)
	}
}

func TestBootstrap_RejectsWrongSecret(t *testing.T) {
	r, _ := authTestSetup(t)
	w := jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": "nope", "username": "root", "password": "pw",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBootstrap_RejectsSecondCall(t *testing.T) {
	r, _ := authTestSetup(t)
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "root", "password": "pw",
	})
	w := jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "root2", "password": "pw2",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLogin_Success(t *testing.T) {
	r, _ := authTestSetup(t)
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})
	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "hunter2",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
	}
	var got loginBody
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.SessionToken == "" {
		t.Fatal("empty session_token on login")
	}
	if !strings.HasPrefix(got.SessionToken, optoken.UserSessionPrefix) {
		t.Errorf("SessionToken missing pst_ prefix: %q", got.SessionToken)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	r, _ := authTestSetup(t)
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})
	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "wrong",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d; want 401", w.Code)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	r, _ := authTestSetup(t)
	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "nobody", "password": "x",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d; want 401 (no username enumeration)", w.Code)
	}
}

func TestRefresh_DeprecationGone(t *testing.T) {
	r, _ := authTestSetup(t)
	w := jsonReq(t, r, "POST", "/api/v1/auth/refresh", map[string]string{
		"refresh_token": "anything",
	})
	if w.Code != http.StatusGone {
		t.Errorf("status=%d, want 410 Gone", w.Code)
	}
}

func TestLogout_RevokesSession(t *testing.T) {
	r, db := authTestSetup(t)
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})
	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "hunter2",
	})
	var login loginBody
	_ = json.NewDecoder(w.Body).Decode(&login)

	// Logout authenticated as the session it's revoking.
	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+login.SessionToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Session row marked revoked.
	s, _ := db.AuthTokens().GetSession(context.Background(), login.TokenID)
	if !s.Revoked {
		t.Errorf("session not revoked after logout")
	}

	// Re-using the bearer must now fail (cache invalidate is synchronous
	// and DB has revoked_at set).
	req2 := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	req2.Header.Set("Authorization", "Bearer "+login.SessionToken)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("post-logout reuse status=%d, want 401", rec2.Code)
	}
}

func TestListSessions_OnlyOwnSessions(t *testing.T) {
	r, _ := authTestSetup(t)
	// Create two users via bootstrap+login dance: one alice, plus a
	// second login from alice's account (simulating another device).
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})
	first := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "hunter2",
	})
	var firstBody loginBody
	_ = json.NewDecoder(first.Body).Decode(&firstBody)

	second := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "hunter2",
	})
	var secondBody loginBody
	_ = json.NewDecoder(second.Body).Decode(&secondBody)

	// Authenticated GET /sessions from the second token must show
	// both sessions, with the calling one tagged current=true.
	req := httptest.NewRequest("GET", "/api/v1/auth/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+secondBody.SessionToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sessions status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Sessions []struct {
			TokenID string `json:"token_id"`
			Current bool   `json:"current"`
		} `json:"sessions"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	// 3 sessions total: bootstrap (auto-issued) + 2 explicit logins.
	if len(resp.Sessions) != 3 {
		t.Errorf("sessions len=%d, want 3 (bootstrap + 2 logins)", len(resp.Sessions))
	}
	for _, s := range resp.Sessions {
		if s.TokenID == secondBody.TokenID && !s.Current {
			t.Errorf("calling session not marked current")
		}
		if s.TokenID == firstBody.TokenID && s.Current {
			t.Errorf("other session incorrectly marked current")
		}
	}
}
