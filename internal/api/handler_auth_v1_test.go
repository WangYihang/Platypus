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

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

const bootstrapSecret = "bootstrap-secret"

// authTestSetup brings up a router that only has the auth routes mounted.
// Returns the router, the storage DB (so tests can seed users directly),
// and the token issuer (for hand-minted edge-case tokens).
func authTestSetup(t *testing.T) (*gin.Engine, *storage.DB, *TokenIssuer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	issuer, err := NewTokenIssuer("access-key", "refresh-key", 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}

	h := NewAuthHandler(db, issuer, bootstrapSecret)
	r := gin.New()
	g := r.Group("/api/v1/auth")
	g.POST("/bootstrap", h.Bootstrap)
	g.POST("/login", h.Login)
	g.POST("/refresh", h.Refresh)
	g.POST("/logout", h.Logout)
	return r, db, issuer
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

type tokenPairBody struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         struct {
		ID       string    `json:"id"`
		Username string    `json:"username"`
		Role     user.Role `json:"role"`
	} `json:"user"`
}

func TestBootstrap_CreatesFirstAdmin(t *testing.T) {
	r, db, _ := authTestSetup(t)

	w := jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret":   bootstrapSecret,
		"username": "root",
		"password": "correct horse battery staple",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("Bootstrap status=%d body=%s", w.Code, w.Body.String())
	}

	var got tokenPairBody
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AccessToken == "" || got.RefreshToken == "" {
		t.Fatalf("empty tokens in response: %+v", got)
	}
	if got.User.Username != "root" || got.User.Role != user.RoleAdmin {
		t.Fatalf("user mismatch: %+v", got.User)
	}

	n, _ := db.Users().Count(context.Background())
	if n != 1 {
		t.Fatalf("user count = %d; want 1", n)
	}
}

func TestBootstrap_RejectsWrongSecret(t *testing.T) {
	r, _, _ := authTestSetup(t)

	w := jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret":   "nope",
		"username": "root",
		"password": "pw",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestBootstrap_RejectsSecondCall(t *testing.T) {
	r, _, _ := authTestSetup(t)

	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "root", "password": "pw",
	})
	// Second call should be refused — a user already exists.
	w := jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "root2", "password": "pw2",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestLogin_Success(t *testing.T) {
	r, _, _ := authTestSetup(t)
	// Use Bootstrap to create the first user so we exercise the real hashing path.
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})

	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice",
		"password": "hunter2",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
	}
	var got tokenPairBody
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got.AccessToken == "" {
		t.Fatal("empty access_token on login")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	r, _, _ := authTestSetup(t)
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})

	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice",
		"password": "wrong",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d; want 401", w.Code)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	r, _, _ := authTestSetup(t)
	w := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "nobody",
		"password": "x",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d; want 401 (no username enumeration)", w.Code)
	}
}

func TestRefresh_RotatesAndRevokesOld(t *testing.T) {
	r, db, _ := authTestSetup(t)
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})
	w1 := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "hunter2",
	})
	var first tokenPairBody
	_ = json.NewDecoder(w1.Body).Decode(&first)

	w2 := jsonReq(t, r, "POST", "/api/v1/auth/refresh", map[string]string{
		"refresh_token": first.RefreshToken,
	})
	if w2.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", w2.Code, w2.Body.String())
	}
	var second tokenPairBody
	_ = json.NewDecoder(w2.Body).Decode(&second)
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh returned the same refresh_token — rotation missing")
	}

	// Re-using the first refresh token must fail now (it's revoked).
	w3 := jsonReq(t, r, "POST", "/api/v1/auth/refresh", map[string]string{
		"refresh_token": first.RefreshToken,
	})
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("stale refresh status=%d; want 401", w3.Code)
	}

	// And the new one is usable.
	w4 := jsonReq(t, r, "POST", "/api/v1/auth/refresh", map[string]string{
		"refresh_token": second.RefreshToken,
	})
	if w4.Code != http.StatusOK {
		t.Fatalf("second refresh status=%d body=%s", w4.Code, w4.Body.String())
	}

	// Sanity: the DB really has the revocation.
	n, _ := db.Users().Count(context.Background())
	if n != 1 {
		t.Fatalf("user count drift: %d", n)
	}
}

func TestLogout_RevokesRefreshToken(t *testing.T) {
	r, _, _ := authTestSetup(t)
	jsonReq(t, r, "POST", "/api/v1/auth/bootstrap", map[string]string{
		"secret": bootstrapSecret, "username": "alice", "password": "hunter2",
	})
	w1 := jsonReq(t, r, "POST", "/api/v1/auth/login", map[string]string{
		"username": "alice", "password": "hunter2",
	})
	var pair tokenPairBody
	_ = json.NewDecoder(w1.Body).Decode(&pair)

	w2 := jsonReq(t, r, "POST", "/api/v1/auth/logout", map[string]string{
		"refresh_token": pair.RefreshToken,
	})
	if w2.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", w2.Code, w2.Body.String())
	}

	// Post-logout, the refresh token is dead.
	w3 := jsonReq(t, r, "POST", "/api/v1/auth/refresh", map[string]string{
		"refresh_token": pair.RefreshToken,
	})
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status=%d; want 401", w3.Code)
	}
}
