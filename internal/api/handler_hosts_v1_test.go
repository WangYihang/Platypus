package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func hostsTestSetup(t *testing.T) (*gin.Engine, *storage.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBAC(db, verifier)
	h := NewHostsHandler(db)

	r := gin.New()
	RegisterV1HostsRoutes(r, h, rbac)
	return r, db
}

func TestHosts_ListEmpty(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/hosts", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Hosts []hostResponse `json:"hosts"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Hosts) != 0 {
		t.Fatalf("empty list should return []; got %+v", resp.Hosts)
	}
}

func TestHosts_ListFiltersByProject(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	prod := seedProjectForAPITest(t, db, "prod", admin)
	stag := seedProjectForAPITest(t, db, "staging", admin)

	now := time.Now().UTC()
	ctx := context.Background()
	_, _ = db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: prod.ID, MachineID: "m-1", Fingerprint: "fp-1",
		Hostname: "alpha", OS: "linux", SeenAt: now,
	})
	_, _ = db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: stag.ID, MachineID: "m-2", Fingerprint: "fp-2",
		Hostname: "beta", OS: "linux", SeenAt: now,
	})

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+prod.ID+"/hosts", tok, nil)
	var resp struct {
		Hosts []hostResponse `json:"hosts"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Hosts) != 1 || resp.Hosts[0].Hostname != "alpha" {
		t.Fatalf("hosts: %+v", resp.Hosts)
	}
}

func TestHosts_ViewerBlocked_NonMember(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	stranger := seedUserForAPITest(t, db, "stranger", user.RoleViewer)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	tok := mintBearerForUserID(t, db, stranger.ID, user.RoleViewer)
	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/hosts", tok, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d; want 403", w.Code)
	}
}

func TestHosts_Get404(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/hosts/missing-id", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d; want 404", w.Code)
	}
}

// A host in one project is not accessible via another project's URL.
func TestHosts_GetCrossProjectIsolated(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	prod := seedProjectForAPITest(t, db, "prod", admin)
	stag := seedProjectForAPITest(t, db, "staging", admin)

	now := time.Now().UTC()
	h, _ := db.Hosts().Upsert(context.Background(), &storage.HostIdentity{
		ProjectID: prod.ID, MachineID: "m-1", Fingerprint: "fp-1",
		Hostname: "alpha", OS: "linux", SeenAt: now,
	})
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	// GET stag/hosts/<prod_host_id> must 404.
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+stag.ID+"/hosts/"+h.ID, tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d; want 404 (cross-project leak)", w.Code)
	}
}
