package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// fakeLive stands in for the real core.TCPServer lifecycle during unit
// tests. It records create/delete calls so assertions stay deterministic
// and doesn't touch the network.
type fakeLive struct {
	mu      sync.Mutex
	created map[string]struct {
		host      string
		port      uint16
		projectID string
	}
	deletes   []string
	createErr error
}

func newFakeLive() *fakeLive {
	return &fakeLive{created: map[string]struct {
		host      string
		port      uint16
		projectID string
	}{}}
}

func (f *fakeLive) Create(host string, port uint16, projectID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return "", f.createErr
	}
	id := fmt.Sprintf("lis-%s-%d", host, port)
	if _, exists := f.created[id]; exists {
		return "", errors.New("port already bound")
	}
	f.created[id] = struct {
		host      string
		port      uint16
		projectID string
	}{host, port, projectID}
	return id, nil
}

func (f *fakeLive) Delete(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.created[id]; !exists {
		return errors.New("not found")
	}
	delete(f.created, id)
	f.deletes = append(f.deletes, id)
	return nil
}

func listenersV2TestSetup(t *testing.T) (*gin.Engine, *storage.DB, *TokenIssuer, *fakeLive) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	issuer, _ := NewTokenIssuer("a", "b", 5*time.Minute, time.Hour)
	rbac := NewRBACWithStorage(issuer, db)
	live := newFakeLive()
	h := NewListenersV2Handler(db, live)

	r := gin.New()
	RegisterV1ProjectListenersRoutes(r, h, rbac)
	return r, db, issuer, live
}

func TestListenersV2_CreatePersistsAndStartsLive(t *testing.T) {
	r, db, issuer, live := listenersV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})

	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/listeners", tok,
		map[string]any{"host": "0.0.0.0", "port": 13337})
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	// DB row landed.
	rows, _ := db.Listeners().ListByProject(context.Background(), proj.ID)
	if len(rows) != 1 || rows[0].Port != 13337 {
		t.Fatalf("rows: %+v", rows)
	}
	// Live side saw the create.
	if len(live.created) != 1 {
		t.Fatalf("live create not invoked: %+v", live.created)
	}
	// The persisted id matches the live id.
	for id := range live.created {
		if rows[0].ID != id {
			t.Fatalf("id mismatch: row=%s live=%s", rows[0].ID, id)
		}
	}
}

func TestListenersV2_OperatorCanCreate(t *testing.T) {
	r, db, issuer, _ := listenersV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	bob := seedUserForAPITest(t, db, "bob", user.RoleOperator)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	_ = db.Projects().AddMember(context.Background(), proj.ID, bob.ID, user.RoleOperator)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: bob.ID, Role: user.RoleOperator})
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/listeners", tok,
		map[string]any{"host": "0.0.0.0", "port": 13338})
	if w.Code != http.StatusCreated {
		t.Fatalf("operator create: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListenersV2_ViewerCannotCreate(t *testing.T) {
	r, db, issuer, _ := listenersV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	bob := seedUserForAPITest(t, db, "bob", user.RoleViewer)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	_ = db.Projects().AddMember(context.Background(), proj.ID, bob.ID, user.RoleViewer)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: bob.ID, Role: user.RoleViewer})
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/listeners", tok,
		map[string]any{"host": "0.0.0.0", "port": 13339})
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer status=%d; want 403", w.Code)
	}
}

func TestListenersV2_LiveCreateFailureDoesNotPersist(t *testing.T) {
	r, db, issuer, live := listenersV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	live.createErr = errors.New("bind: permission denied")

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/listeners", tok,
		map[string]any{"host": "0.0.0.0", "port": 80})
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status=%d; want 502", w.Code)
	}
	// No row should have been written.
	rows, _ := db.Listeners().ListByProject(context.Background(), proj.ID)
	if len(rows) != 0 {
		t.Fatalf("row persisted despite live failure: %+v", rows)
	}
}

func TestListenersV2_ListAndDelete(t *testing.T) {
	r, db, issuer, live := listenersV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})

	probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/listeners", tok,
		map[string]any{"host": "0.0.0.0", "port": 13337})
	probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/listeners", tok,
		map[string]any{"host": "0.0.0.0", "port": 13338})

	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/listeners", tok, nil)
	var listResp struct {
		Listeners []struct {
			ID   string `json:"id"`
			Port uint16 `json:"port"`
		} `json:"listeners"`
	}
	_ = json.NewDecoder(w.Body).Decode(&listResp)
	if len(listResp.Listeners) != 2 {
		t.Fatalf("list: %+v", listResp.Listeners)
	}

	// Delete the first.
	first := listResp.Listeners[0].ID
	w = probeReqWithPath(r, "DELETE", "/api/v1/projects/"+proj.ID+"/listeners/"+first, tok, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", w.Code, w.Body.String())
	}
	// Live delete called.
	if len(live.deletes) != 1 || live.deletes[0] != first {
		t.Fatalf("live deletes: %+v", live.deletes)
	}
	// DB row gone.
	if _, err := db.Listeners().GetByID(context.Background(), first); err != storage.ErrNotFound {
		t.Fatalf("row still present: %v", err)
	}
}

// Cross-project delete: a listener from project A can't be deleted via
// project B's URL. Prevents the URL from leaking the existence of live
// listeners across tenants.
func TestListenersV2_DeleteCrossProjectBlocked(t *testing.T) {
	r, db, issuer, _ := listenersV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	prod := seedProjectForAPITest(t, db, "prod", admin)
	stag := seedProjectForAPITest(t, db, "staging", admin)
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})

	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+prod.ID+"/listeners", tok,
		map[string]any{"host": "0.0.0.0", "port": 13337})
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&created)

	// Try to delete via staging's URL.
	w = probeReqWithPath(r, "DELETE",
		"/api/v1/projects/"+stag.ID+"/listeners/"+created.ID, tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project delete status=%d; want 404", w.Code)
	}
}
