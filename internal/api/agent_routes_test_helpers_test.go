package api

import (
	"context"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// agentRouteFixture bundles everything an agent-route test needs to
// drive the new project-scoped endpoints under
// /api/v1/projects/:pid/agents/:agent_id/...
//
// It seeds an admin user, a project, and a host row stamped with the
// supplied agent_id so RequireAgentInProject finds the host and matches
// it to the project. Tests build URLs as
// fixture.URL("/fs/list?path=/tmp") to keep the prefix in one place.
type agentRouteFixture struct {
	DB        *storage.DB
	Issuer    *TokenIssuer
	RBAC      *RBAC
	Token     string
	ProjectID string
	AgentID   string
	prefix    string
}

// newAgentRouteFixture wires the RBAC dependencies and seeds the
// minimum DB rows. It does NOT register the agent in the link service —
// callers do that separately because the test helpers in each file
// build paired link.Sessions on the way in.
func newAgentRouteFixture(t *testing.T, agentID string) *agentRouteFixture {
	t.Helper()

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	issuer, err := NewTokenIssuer("a", "b", 5*time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}

	admin := seedUserForAPITest(t, db, "agent-route-admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "agent-route-prod", admin)

	if _, err := db.Hosts().Upsert(context.Background(), &storage.HostIdentity{
		ProjectID:   proj.ID,
		MachineID:   "m-" + agentID,
		Fingerprint: "fp-" + agentID,
		Hostname:    "host-" + agentID,
		OS:          "linux",
		SeenAt:      time.Now().UTC(),
		AgentID:     agentID,
	}); err != nil {
		t.Fatalf("seed host: %v", err)
	}

	tok, err := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}

	return &agentRouteFixture{
		DB:        db,
		Issuer:    issuer,
		RBAC:      NewRBACWithStorage(issuer, db),
		Token:     tok,
		ProjectID: proj.ID,
		AgentID:   agentID,
		prefix:    "/api/v1/projects/" + proj.ID + "/agents/" + agentID,
	}
}

// URL composes a path under the fixture's project-scoped agent prefix.
// suffix should start with a "/", e.g. "/fs/list?path=/tmp".
func (f *agentRouteFixture) URL(suffix string) string {
	return f.prefix + suffix
}
