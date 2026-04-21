package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func sessionsV2TestSetup(t *testing.T) (*gin.Engine, *storage.DB, *TokenIssuer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	issuer, _ := NewTokenIssuer("a", "b", 5*time.Minute, time.Hour)
	rbac := NewRBACWithStorage(issuer, db)
	h := NewSessionsV2Handler(db)

	r := gin.New()
	RegisterV1ProjectSessionsRoutes(r, h, rbac)
	return r, db, issuer
}

// seedSessionRow inserts the minimum set of rows (host + listener +
// session) and returns the storage objects the test can reference.
func seedSessionRow(t *testing.T, db *storage.DB, project *storage.Project, sessionID string, disconnect bool) (*storage.Host, *storage.Listener, *storage.Session) {
	t.Helper()
	ctx := context.Background()
	host, err := db.Hosts().Upsert(ctx, &storage.HostIdentity{
		ProjectID: project.ID, MachineID: "m-" + sessionID, Fingerprint: "fp-" + sessionID,
		Hostname: "host-" + sessionID, OS: "linux", SeenAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("host upsert: %v", err)
	}
	lis := &storage.Listener{
		ID:        "lis-" + sessionID,
		ProjectID: project.ID,
		Host:      "0.0.0.0", Port: 13337, CreatedAt: time.Now().UTC(),
	}
	_ = db.Listeners().Create(ctx, lis)
	sess := &storage.Session{
		ID: sessionID, ProjectID: project.ID, ListenerID: lis.ID, HostID: host.ID,
		User: "root", ConnectedAt: time.Now().UTC(),
	}
	_ = db.Sessions().Insert(ctx, sess)
	if disconnect {
		_ = db.Sessions().MarkDisconnected(ctx, sessionID)
	}
	return host, lis, sess
}

func TestSessionsV2_ListForHost_IncludesHistorical(t *testing.T) {
	r, db, issuer := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	// Seed two sessions on the same host: one still live, one already
	// disconnected. The response must include both, newest-connected
	// first.
	host, lis, _ := seedSessionRow(t, db, proj, "s-live", false)
	ctx := context.Background()
	_ = db.Sessions().Insert(ctx, &storage.Session{
		ID: "s-dead", ProjectID: proj.ID, ListenerID: lis.ID, HostID: host.ID,
		ConnectedAt: time.Now().UTC(),
	})
	_ = db.Sessions().MarkDisconnected(ctx, "s-dead")

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
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
	r, db, issuer := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	prod := seedProjectForAPITest(t, db, "prod", admin)
	stag := seedProjectForAPITest(t, db, "staging", admin)

	// Host in prod.
	host, _, _ := seedSessionRow(t, db, prod, "s-1", false)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+stag.ID+"/hosts/"+host.ID+"/sessions", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project host status=%d; want 404", w.Code)
	}
}

func TestSessionsV2_Dispatch_NoLiveSessions_EmptyResults(t *testing.T) {
	r, db, issuer := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	// No live flagged sessions -> empty result, not an error.
	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
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

// A session whose runtime is missing (row live but no AgentClient in the
// registry — i.e. a server restart while a session was alive) surfaces as
// an error row, not a 500.
func TestSessionsV2_Dispatch_RuntimeMissing_ReturnsErrorRow(t *testing.T) {
	r, db, issuer := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "prod", admin)

	// Insert a live session marked group_dispatch=true with no matching
	// runtime AgentClient (core.FindAgentClientByHash returns nil).
	host, lis, _ := seedSessionRow(t, db, proj, "s-orphan", false)
	_ = db.Sessions().SetGroupDispatch(context.Background(), "s-orphan", true)
	_ = host
	_ = lis

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
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
	if resp.Count != 1 || resp.Results[0].Error == "" {
		t.Fatalf("expected one error-row result; got %+v", resp)
	}
}

func TestSessionsV2_Dispatch_ViewerBlocked(t *testing.T) {
	r, db, issuer := sessionsV2TestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	bob := seedUserForAPITest(t, db, "bob", user.RoleViewer)
	proj := seedProjectForAPITest(t, db, "prod", admin)
	_ = db.Projects().AddMember(context.Background(), proj.ID, bob.ID, user.RoleViewer)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: bob.ID, Role: user.RoleViewer})
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/dispatch", tok,
		map[string]any{"command": "id"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer dispatch status=%d; want 403", w.Code)
	}
}
