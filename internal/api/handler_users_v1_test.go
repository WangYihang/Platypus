package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func usersTestSetup(t *testing.T) (*gin.Engine, *storage.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rb := NewRBAC(db, verifier)
	h := NewUsersHandler(db)

	r := gin.New()
	g := r.Group("/api/v1/users")
	g.Use(rb.RequireAuth(), rb.RequireGlobalRole(user.RoleAdmin))
	{
		g.GET("", h.List)
		g.POST("", h.Create)
		g.GET("/:id", h.Get)
		g.PATCH("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
	}
	return r, db
}

// tokenFor seeds a synthetic user (id="tester-<role>") and returns
// their session bearer. The user is created idempotently so callers
// can ask for the same role twice without collision.
func tokenFor(t *testing.T, db *storage.DB, role user.Role) string {
	t.Helper()
	return mintBearerForUserID(t, db, "tester-"+string(role), role)
}

func TestUsersList_AdminOnly(t *testing.T) {
	r, db := usersTestSetup(t)

	// Viewer → 403
	w := probeReqWithPath(r, "GET", "/api/v1/users", tokenFor(t, db, user.RoleViewer), nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer: status=%d; want 403", w.Code)
	}

	// Admin → 200
	w = probeReqWithPath(r, "GET", "/api/v1/users", tokenFor(t, db, user.RoleAdmin), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("admin: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestUsersCreateAndList(t *testing.T) {
	r, db := usersTestSetup(t)
	admin := tokenFor(t, db, user.RoleAdmin)

	w := probeReqWithPath(r, "POST", "/api/v1/users", admin, map[string]string{
		"username": "bob",
		"password": "hunter2",
		"role":     "operator",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", w.Code, w.Body.String())
	}

	w = probeReqWithPath(r, "GET", "/api/v1/users", admin, nil)
	var resp struct {
		Users []userBody `json:"users"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	// tokenFor seeds a tester-admin user under the covers, plus the
	// bob row this test created — list must contain both.
	var bobFound bool
	for _, u := range resp.Users {
		if u.Username == "bob" && u.Role == user.RoleOperator {
			bobFound = true
		}
	}
	if !bobFound {
		t.Fatalf("list missing bob: %+v", resp.Users)
	}
}

func TestUsersCreate_DuplicateUsername(t *testing.T) {
	r, db := usersTestSetup(t)
	admin := tokenFor(t, db, user.RoleAdmin)

	body := map[string]string{"username": "bob", "password": "x", "role": "operator"}
	w := probeReqWithPath(r, "POST", "/api/v1/users", admin, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create status=%d", w.Code)
	}
	w = probeReqWithPath(r, "POST", "/api/v1/users", admin, body)
	if w.Code != http.StatusConflict {
		t.Fatalf("second create status=%d; want 409", w.Code)
	}
}

func TestUsersCreate_InvalidRole(t *testing.T) {
	r, db := usersTestSetup(t)
	admin := tokenFor(t, db, user.RoleAdmin)

	w := probeReqWithPath(r, "POST", "/api/v1/users", admin, map[string]string{
		"username": "bob", "password": "x", "role": "superuser",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d; want 400", w.Code)
	}
}

func TestUsersUpdateRole(t *testing.T) {
	r, db := usersTestSetup(t)
	admin := tokenFor(t, db, user.RoleAdmin)

	probeReqWithPath(r, "POST", "/api/v1/users", admin, map[string]string{
		"username": "bob", "password": "x", "role": "operator",
	})
	bob, _ := db.Users().GetByUsername(testCtx(), "bob")

	w := probeReqWithPath(r, "PATCH", "/api/v1/users/"+bob.ID, admin, map[string]string{
		"role": "viewer",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update: status=%d body=%s", w.Code, w.Body.String())
	}
	got, _ := db.Users().GetByID(testCtx(), bob.ID)
	if got.Role != user.RoleViewer {
		t.Fatalf("role = %q; want viewer", got.Role)
	}
}

func TestUsersDelete(t *testing.T) {
	r, db := usersTestSetup(t)
	admin := tokenFor(t, db, user.RoleAdmin)

	probeReqWithPath(r, "POST", "/api/v1/users", admin, map[string]string{
		"username": "bob", "password": "x", "role": "viewer",
	})
	bob, _ := db.Users().GetByUsername(testCtx(), "bob")

	w := probeReqWithPath(r, "DELETE", "/api/v1/users/"+bob.ID, admin, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: status=%d body=%s", w.Code, w.Body.String())
	}
	if _, err := db.Users().GetByID(testCtx(), bob.ID); err != storage.ErrNotFound {
		t.Fatalf("user still present after delete: err=%v", err)
	}
}

func TestUsersGet_NotFound(t *testing.T) {
	r, db := usersTestSetup(t)
	admin := tokenFor(t, db, user.RoleAdmin)

	w := probeReqWithPath(r, "GET", "/api/v1/users/missing-id", admin, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d; want 404", w.Code)
	}
}
