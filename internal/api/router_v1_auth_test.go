package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
)

// Wiring test: RegisterV1AuthRoutes must mount the auth + users
// endpoints under /api/v1/ so that a real bootstrap → /users flow
// works end-to-end against a single engine, not a test-only subset.
func TestRegisterV1AuthRoutes_BootstrapLoginList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBAC(db, verifier)
	authH := NewAuthHandler(db, verifier, "seekret")
	usersH := NewUsersHandler(db)

	engine := CreateRESTfulAPIServer()
	RegisterV1AuthRoutes(engine, authH, usersH, rbac)

	w := probeReqWithPath(engine, "POST", "/api/v1/auth/bootstrap", "", map[string]string{
		"secret": "seekret", "username": "root", "password": "pw",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bootstrap: status=%d body=%s", w.Code, w.Body.String())
	}
	var login loginBody
	_ = json.NewDecoder(w.Body).Decode(&login)
	if login.SessionToken == "" {
		t.Fatal("no session token from bootstrap")
	}

	w = probeReqWithPath(engine, "GET", "/api/v1/users", login.SessionToken, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /users: status=%d body=%s", w.Code, w.Body.String())
	}

	w = probeReqWithPath(engine, "GET", "/api/v1/users", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauth list: status=%d; want 401", w.Code)
	}
}
