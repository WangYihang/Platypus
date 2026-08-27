package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func seedArchiveHost(t *testing.T, db *storage.DB, projectID, name string) *storage.Host {
	t.Helper()
	h, err := db.Hosts().Upsert(context.Background(), &storage.HostIdentity{
		ProjectID:       projectID,
		MachineID:       "m-" + name,
		Fingerprint:     "fp-" + name,
		Hostname:        name,
		SeenAt:          time.Now().UTC(),
		InitialApproval: storage.HostApprovalApproved,
	})
	if err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
	return h
}

func listHostNames(t *testing.T, r http.Handler, projectID, tok, path string) []string {
	t.Helper()
	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+projectID+path, tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", path, w.Code, w.Body.String())
	}
	var resp struct {
		Hosts []hostResponse `json:"hosts"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	names := make([]string, 0, len(resp.Hosts))
	for _, h := range resp.Hosts {
		names = append(names, h.Hostname)
	}
	return names
}

// The whole point of the feature: DELETE takes a host out of the fleet
// list, and the archived listing is where it turns up instead.
func TestHosts_Archive_RemovesFromListAndAppearsInArchived(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	seedArchiveHost(t, db, proj.ID, "keep-me")
	drop := seedArchiveHost(t, db, proj.ID, "zombie")

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	w := probeReqWithPath(r, "DELETE", "/api/v1/projects/"+proj.ID+"/hosts/"+drop.ID, tok,
		map[string]any{"reason": "decommissioned"})
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE status=%d body=%s", w.Code, w.Body.String())
	}

	live := listHostNames(t, r, proj.ID, tok, "/hosts")
	if len(live) != 1 || live[0] != "keep-me" {
		t.Errorf("fleet list = %v, want only [keep-me]", live)
	}
	archived := listHostNames(t, r, proj.ID, tok, "/hosts/archived")
	if len(archived) != 1 || archived[0] != "zombie" {
		t.Errorf("archived list = %v, want only [zombie]", archived)
	}
}

// Restore is the undo, and it has to put the row back in the list it
// was taken out of.
func TestHosts_Restore_ReturnsToFleetList(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	h := seedArchiveHost(t, db, proj.ID, "back-again")
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	if w := probeReqWithPath(r, "DELETE", "/api/v1/projects/"+proj.ID+"/hosts/"+h.ID, tok, nil); w.Code != http.StatusOK {
		t.Fatalf("DELETE status=%d body=%s", w.Code, w.Body.String())
	}
	if w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/hosts/"+h.ID+"/restore", tok, nil); w.Code != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", w.Code, w.Body.String())
	}

	if live := listHostNames(t, r, proj.ID, tok, "/hosts"); len(live) != 1 || live[0] != "back-again" {
		t.Errorf("fleet list after restore = %v, want [back-again]", live)
	}
	if archived := listHostNames(t, r, proj.ID, tok, "/hosts/archived"); len(archived) != 0 {
		t.Errorf("archived list after restore = %v, want empty", archived)
	}
}

// An operator double-clicking Archive should get a no-op, not a 404
// that reads like the host vanished. Same for Restore.
func TestHosts_Archive_IsIdempotent(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	h := seedArchiveHost(t, db, proj.ID, "clicky")
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	path := "/api/v1/projects/" + proj.ID + "/hosts/" + h.ID
	for i := range 2 {
		if w := probeReqWithPath(r, "DELETE", path, tok, nil); w.Code != http.StatusOK {
			t.Fatalf("DELETE #%d status=%d body=%s", i+1, w.Code, w.Body.String())
		}
	}
	for i := range 2 {
		if w := probeReqWithPath(r, "POST", path+"/restore", tok, nil); w.Code != http.StatusOK {
			t.Fatalf("restore #%d status=%d body=%s", i+1, w.Code, w.Body.String())
		}
	}
}

func TestHosts_Archive_UnknownHostIs404(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	w := probeReqWithPath(r, "DELETE", "/api/v1/projects/"+proj.ID+"/hosts/no-such-host", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404 — body=%s", w.Code, w.Body.String())
	}
}

// Archiving is destructive-looking to anyone reading the fleet list, so
// it sits behind admin like approve/reject do.
func TestHosts_Archive_RequiresAdmin(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	h := seedArchiveHost(t, db, proj.ID, "guarded")

	operator := seedUserForAPITest(t, db, "op", user.RoleOperator)
	opTok := mintBearerForUserID(t, db, operator.ID, user.RoleOperator)

	w := probeReqWithPath(r, "DELETE", "/api/v1/projects/"+proj.ID+"/hosts/"+h.ID, opTok, nil)
	if w.Code == http.StatusOK {
		t.Errorf("operator was allowed to archive a host (status=%d)", w.Code)
	}
}

// "archived" is a literal path segment sharing a prefix with /:hid, so
// pin that it lists rather than being read as a host id.
func TestHosts_ArchivedListing_NotMistakenForHostID(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/hosts/archived", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Hosts []hostResponse `json:"hosts"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Hosts == nil {
		t.Error("expected a hosts array, got null — the route resolved to something else")
	}
}
