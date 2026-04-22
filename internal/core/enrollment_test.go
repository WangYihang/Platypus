package core_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/protocol"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	"github.com/WangYihang/Platypus/internal/utils/config"
	agentpb "github.com/WangYihang/Platypus/pkg/proto/agent/v1"
)

// TestTryEnroll_PAT_HappyPath drives a full enrollment exchange across a
// net.Pipe: the "agent" goroutine sends AgentEnrollRequest with a freshly
// minted PAT, the "server" side runs TryEnroll, and we verify the reply
// carries a well-formed session token plus that the audit tables got a
// success row.
func TestTryEnroll_PAT_HappyPath(t *testing.T) {
	// Shared test fixture: in-memory DB + enrollment service + Ctx wiring.
	db := mustOpenDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", admin)

	svc := enrollment.New(db)
	core.SetEnrollment(svc)
	installCtx(t, db)
	defer uninstallCtx()

	issue, err := svc.MintPAT(ctx, enrollment.MintPATInput{
		ProjectID: proj.ID, IssuedByUser: admin.ID, MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("MintPAT: %v", err)
	}

	// net.Pipe gives us a synchronous in-memory conn pair. The "server"
	// side runs TryEnroll; the "agent" side drives the protobuf frames.
	serverConn, agentConn := net.Pipe()
	defer serverConn.Close()
	defer agentConn.Close()

	// Fake AgentClient pointing at the server end of the pipe.
	client := core.NewAgentClientForTest(serverConn)

	done := make(chan struct {
		out *core.EnrollmentOutcome
		err error
	}, 1)
	go func() {
		out, err := core.TryEnroll(client)
		done <- struct {
			out *core.EnrollmentOutcome
			err error
		}{out, err}
	}()

	// Agent side: send request, read response.
	agentCodec := protocol.NewProtoCodec(agentConn)
	req := &agentpb.Envelope{
		Payload: &agentpb.Envelope_AgentEnrollRequest{
			AgentEnrollRequest: &agentpb.AgentEnrollRequest{
				Credential: issue.PlaintextToken,
				MachineId:  "m-test",
				Hostname:   "h-test",
				Os:         "linux", Arch: "amd64", Version: "test",
			},
		},
	}
	if err := agentCodec.Send(req); err != nil {
		t.Fatalf("Send: %v", err)
	}
	respEnv, err := agentCodec.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	resp, ok := respEnv.Payload.(*agentpb.Envelope_AgentEnrollResponse)
	if !ok {
		t.Fatalf("unexpected payload: %T", respEnv.Payload)
	}
	if resp.AgentEnrollResponse.Error != "" {
		t.Fatalf("server-side error: %s", resp.AgentEnrollResponse.Error)
	}
	if resp.AgentEnrollResponse.AgentId == "" {
		t.Fatal("empty agent_id in response")
	}
	if resp.AgentEnrollResponse.SessionToken == "" {
		t.Fatal("empty session_token in response")
	}

	result := <-done
	if result.err != nil {
		t.Fatalf("TryEnroll err: %v", result.err)
	}
	if !result.out.Succeeded {
		t.Fatalf("TryEnroll Outcome = %q; want success", result.out.Outcome)
	}

	// Audit: PAT redemption row + connection event row must exist.
	evts, err := db.PATRedemptionEvents().ListByToken(ctx, issue.TokenID, 10)
	if err != nil {
		t.Fatalf("ListByToken: %v", err)
	}
	if len(evts) != 1 || evts[0].Outcome != "success" {
		t.Fatalf("redemption events = %+v", evts)
	}
	connEvts, err := db.AgentConnectionEvents().ListByAgent(ctx, result.out.AgentID, 10)
	if err != nil {
		t.Fatalf("ListByAgent: %v", err)
	}
	if len(connEvts) != 1 || connEvts[0].EventType != "enroll_success" {
		t.Fatalf("connection events = %+v", connEvts)
	}
}

// TestTryEnroll_LegacyNoFrame confirms that a silent agent (legacy build
// that doesn't speak enrollment) is tolerated — TryEnroll returns
// Attempted=false so the caller falls back to the legacy handshake.
func TestTryEnroll_LegacyNoFrame(t *testing.T) {
	db := mustOpenDB(t)
	installCtx(t, db)
	defer uninstallCtx()

	core.SetEnrollment(enrollment.New(db))

	serverConn, agentConn := net.Pipe()
	defer serverConn.Close()
	defer agentConn.Close()
	client := core.NewAgentClientForTest(serverConn)

	out, err := core.TryEnroll(client)
	if err != nil {
		t.Fatalf("TryEnroll: %v", err)
	}
	if out.Attempted {
		t.Fatalf("Attempted = true; want false (no frame sent)")
	}
}

// --- helpers -----------------------------------------------------------

func mustOpenDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedUser(t *testing.T, db *storage.DB, name string, role user.Role) *user.User {
	t.Helper()
	u := &user.User{
		ID: "user-" + name, Username: name, PasswordHash: "hash", Role: role,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.Users().Create(context.Background(), u); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	return u
}

func seedProject(t *testing.T, db *storage.DB, slug string, creator *user.User) *storage.Project {
	t.Helper()
	p := &storage.Project{
		ID: "proj-" + slug, Name: slug, Slug: slug,
		CreatedAt: time.Now().UTC(), CreatedBy: creator.ID,
	}
	if err := db.Projects().Create(context.Background(), p); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	return p
}

// installCtx wires a minimal *app.App into core.Ctx so recordConnectionEvent
// has somewhere to write. Restored by uninstallCtx via Cleanup.
func installCtx(t *testing.T, db *storage.DB) {
	t.Helper()
	prev := core.Ctx
	core.Ctx = app.New(&config.Config{})
	core.Ctx.Storage = db
	t.Cleanup(func() { core.Ctx = prev })
}

func uninstallCtx() {}
