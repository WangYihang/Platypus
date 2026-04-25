package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func sessionsV2TestSetup(t *testing.T) (*gin.Engine, *storage.DB, *core.AgentLinkService) {
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
	links := core.NewAgentLinkService()
	h := NewSessionsV2Handler(db, links)

	r := gin.New()
	RegisterV1ProjectSessionsRoutes(r, h, rbac)
	return r, db, links
}

const testIngressAddr = "0.0.0.0:9443"

// seedSessionRow inserts the minimum set of rows (host + session) and
// returns the storage objects the test can reference. The host gets
// agent_id="agent-<sessionID>" stamped so resolveLiveAgentForSession
// can map session → host → agent without relying on a separate
// enrollment fixture.
func seedSessionRow(t *testing.T, db *storage.DB, project *storage.Project, sessionID string, disconnect bool) (*storage.Host, *storage.Session) {
	t.Helper()
	ctx := context.Background()
	host, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: project.ID, MachineID: "m-" + sessionID, Fingerprint: "fp-" + sessionID,
		Hostname: "host-" + sessionID, OS: "linux", SeenAt: time.Now().UTC(),
		AgentID: "agent-" + sessionID,
	})
	if err != nil {
		t.Fatalf("host upsert: %v", err)
	}
	sess := &storage.Session{
		ID: sessionID, ProjectID: project.ID, IngressAddr: testIngressAddr, HostID: host.ID,
		User: "root", ConnectedAt: time.Now().UTC(),
	}
	_ = db.Sessions().Insert(ctx, sess)
	if disconnect {
		_ = db.Sessions().MarkDisconnected(ctx, sessionID)
	}
	return host, sess
}

func TestSessionsV2_ListForHost_IncludesHistorical(t *testing.T) {
	r, db, _ := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	// Seed two sessions on the same host: one still live, one already
	// disconnected. The response must include both, newest-connected
	// first.
	host, _ := seedSessionRow(t, db, proj, "s-live", false)
	ctx := context.Background()
	_ = db.Sessions().Insert(ctx, &storage.Session{
		ID: "s-dead", ProjectID: proj.ID, IngressAddr: testIngressAddr, HostID: host.ID,
		ConnectedAt: time.Now().UTC(),
	})
	_ = db.Sessions().MarkDisconnected(ctx, "s-dead")

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/hosts/"+host.ID+"/sessions", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Sessions []sessionResponse `json:"sessions"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions (1 live + 1 historical); got %d", len(resp.Sessions))
	}
}

func TestSessionsV2_ListForHost_CrossProject404(t *testing.T) {
	r, db, _ := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	prod := seedProjectForAPITest(t, db, "prod", admin)
	stag := seedProjectForAPITest(t, db, "staging", admin)

	// Host in prod.
	host, _ := seedSessionRow(t, db, prod, "s-1", false)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+stag.ID+"/hosts/"+host.ID+"/sessions", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project host status=%d; want 404", w.Code)
	}
}

func TestSessionsV2_ListForProject_FiltersLiveAndSince(t *testing.T) {
	r, db, links := sessionsV2TestSetup(t)
	ctx := context.Background()
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	// One closed session 2 days ago; one live now. The live row's
	// agent must be registered with AgentLinkService — that's the
	// SoT for liveness now, and the handler intersects the DB rows
	// against it. nil session pointer is fine for presence-only.
	host, _ := seedSessionRow(t, db, proj, "live-now", false)
	links.Register("agent-live-now", nil)
	_ = db.Sessions().Insert(ctx, &storage.Session{
		ID: "old-closed", ProjectID: proj.ID, IngressAddr: testIngressAddr, HostID: host.ID,
		ConnectedAt: time.Now().UTC().Add(-48 * time.Hour),
	})
	_ = db.Sessions().MarkDisconnected(ctx, "old-closed")

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	// No filter → both rows.
	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/sessions", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("base status=%d body=%s", w.Code, w.Body.String())
	}
	var all struct {
		Sessions []sessionResponse `json:"sessions"`
	}
	_ = json.NewDecoder(w.Body).Decode(&all)
	if len(all.Sessions) != 2 {
		t.Fatalf("expected 2 sessions; got %d", len(all.Sessions))
	}

	// live=true → just the live one.
	w = probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/sessions?live=true", tok, nil)
	var liveOnly struct {
		Sessions []sessionResponse `json:"sessions"`
	}
	_ = json.NewDecoder(w.Body).Decode(&liveOnly)
	if len(liveOnly.Sessions) != 1 || liveOnly.Sessions[0].ID != "live-now" {
		t.Fatalf("live filter: %+v", liveOnly.Sessions)
	}

	// since=24h ago → drops the 48h-old row.
	since := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	w = probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/sessions?since="+since, tok, nil)
	var recent struct {
		Sessions []sessionResponse `json:"sessions"`
	}
	_ = json.NewDecoder(w.Body).Decode(&recent)
	if len(recent.Sessions) != 1 || recent.Sessions[0].ID != "live-now" {
		t.Fatalf("since filter: %+v", recent.Sessions)
	}

	// Bogus live value → 400.
	w = probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/sessions?live=maybe", tok, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad live value status=%d; want 400", w.Code)
	}
}

func TestSessionsV2_Dispatch_NoLiveSessions_EmptyResults(t *testing.T) {
	r, db, _ := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	// No live flagged sessions -> empty result, not an error.
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/dispatch", tok,
		map[string]any{"command": "id", "timeout": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Count   int                `json:"count"`
		Results []dispatchV2Result `json:"results"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 0 || len(resp.Results) != 0 {
		t.Fatalf("expected empty results; got %+v", resp)
	}
}

// A "live" sessions row whose agent isn't registered in the
// in-memory AgentLinkService is an audit-tail leak — typically a
// previous server instance crashed before stamping disconnected_at.
// Dispatch silently skips the row rather than producing a confusing
// "session runtime missing" error: the SSOT model says liveness is
// owned by AgentLinkService, and an unregistered agent simply isn't
// live, regardless of what the DB still has open.
func TestSessionsV2_Dispatch_AuditTailRow_SilentlySkipped(t *testing.T) {
	r, db, _ := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	// Open audit row marked for group dispatch, but no matching
	// registration in AgentLinkService — exactly the post-crash
	// shape the startup audit-close sweep eventually fixes.
	_, _ = seedSessionRow(t, db, proj, "s-zombie", false)
	_ = db.Sessions().SetGroupDispatch(context.Background(), "s-zombie", true)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/dispatch", tok,
		map[string]any{"command": "id", "timeout": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Count   int                `json:"count"`
		Results []dispatchV2Result `json:"results"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Count != 0 || len(resp.Results) != 0 {
		t.Fatalf("audit-tail row should be skipped; got %+v", resp)
	}
}

// The intersection between DB-open rows and AgentLinkService
// registrations is what backs ?live=true. Two rows seeded here have
// disconnected_at IS NULL, but only one has a registered agent —
// the response must drop the unregistered (audit-tail) row.
func TestSessionsV2_ListForProject_LiveFilter_ExcludesUnregisteredAgents(t *testing.T) {
	r, db, links := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	// Two open rows on different hosts. The seed helper stamps
	// host.agent_id="agent-<sessionID>", so we can register one and
	// leave the other absent.
	_, _ = seedSessionRow(t, db, proj, "s-registered", false)
	_, _ = seedSessionRow(t, db, proj, "s-zombie", false)
	links.Register("agent-s-registered", nil)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/sessions?live=true", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Sessions []sessionResponse `json:"sessions"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Sessions) != 1 || resp.Sessions[0].ID != "s-registered" {
		t.Fatalf("expected only the registered row; got %+v", resp.Sessions)
	}
}

func TestSessionsV2_Dispatch_ViewerBlocked(t *testing.T) {
	r, db, _ := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	bob := seedUserForAPITest(t, db, "bob", user.RoleViewer)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	_ = db.Projects().AddMember(context.Background(), proj.ID, bob.ID, user.RoleViewer)

	tok := mintBearerForUserID(t, db, bob.ID, user.RoleViewer)
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/dispatch", tok,
		map[string]any{"command": "id"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer dispatch status=%d; want 403", w.Code)
	}
}
