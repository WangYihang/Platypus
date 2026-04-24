package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/settings"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func adminSettingsTestSetup(t *testing.T) (*gin.Engine, *storage.DB, *TokenIssuer, *settings.Registry) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	issuer, _ := NewTokenIssuer("a", "b", 5*time.Minute, time.Hour)
	rbac := NewRBACWithStorage(issuer, db)
	reg := settings.New(db, nil)
	h := NewAdminSettingsHandler(reg)

	r := gin.New()
	RegisterV1AdminSettingsRoutes(r, h, rbac)
	return r, db, issuer, reg
}

func TestAdminSettings_ListReturns6Descriptors(t *testing.T) {
	r, db, issuer, _ := adminSettingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})

	w := probeReqWithPath(r, "GET", "/api/v1/admin/settings", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Settings []settings.SettingDescriptor `json:"settings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Settings) != 6 {
		t.Fatalf("len=%d want=6 body=%s", len(body.Settings), w.Body.String())
	}
}

func TestAdminSettings_UpdatePersistsAndInvalidatesCache(t *testing.T) {
	r, db, issuer, reg := adminSettingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})

	w := probeReqWithPath(r, "PUT", "/api/v1/admin/settings/"+settings.KeyAuthAccessTokenTTL, tok,
		map[string]int{"value": 60})
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	// Registry should reflect the new value immediately.
	if got := reg.AccessTokenTTL(); got != 60*time.Second {
		t.Errorf("registry TTL = %v, want 60s", got)
	}
}

func TestAdminSettings_UpdateRejectsBadType(t *testing.T) {
	r, db, issuer, _ := adminSettingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})

	// mesh.discovery_lan is a bool; sending a string should fail validation.
	w := probeReqWithPath(r, "PUT", "/api/v1/admin/settings/"+settings.KeyMeshDiscoveryLAN, tok,
		map[string]string{"value": "nope"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminSettings_UpdateRejectsUnknownKey(t *testing.T) {
	r, db, issuer, _ := adminSettingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})

	w := probeReqWithPath(r, "PUT", "/api/v1/admin/settings/not.a.key", tok,
		map[string]int{"value": 1})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminSettings_NonAdminForbidden(t *testing.T) {
	r, db, issuer, _ := adminSettingsTestSetup(t)
	viewer := seedUserForAPITest(t, db, "viewer", user.RoleViewer)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: viewer.ID, Role: user.RoleViewer})

	w := probeReqWithPath(r, "GET", "/api/v1/admin/settings", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("GET status=%d", w.Code)
	}

	w = probeReqWithPath(r, "PUT", "/api/v1/admin/settings/"+settings.KeyAuthAccessTokenTTL, tok,
		map[string]int{"value": 60})
	if w.Code != http.StatusForbidden {
		t.Fatalf("PUT status=%d", w.Code)
	}
}

func TestAdminSettings_UnauthenticatedRejected(t *testing.T) {
	r, _, _, _ := adminSettingsTestSetup(t)
	w := probeReqWithPath(r, "GET", "/api/v1/admin/settings", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestAdminSettings_ResetRevertsToDefault(t *testing.T) {
	r, db, issuer, reg := adminSettingsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})

	// Seed an override.
	w := probeReqWithPath(r, "PUT", "/api/v1/admin/settings/"+settings.KeyAuthAccessTokenTTL, tok,
		map[string]int{"value": 60})
	if w.Code != http.StatusNoContent {
		t.Fatalf("precondition set: %d", w.Code)
	}
	if reg.AccessTokenTTL() != 60*time.Second {
		t.Fatal("precondition: registry didn't see the override")
	}

	// Reset drops it.
	w = probeReqWithPath(r, "DELETE", "/api/v1/admin/settings/"+settings.KeyAuthAccessTokenTTL, tok, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("reset status=%d body=%s", w.Code, w.Body.String())
	}
	if got := reg.AccessTokenTTL(); got != settings.DefaultAccessTokenTTL {
		t.Errorf("after reset = %v, want default %v", got, settings.DefaultAccessTokenTTL)
	}
}
