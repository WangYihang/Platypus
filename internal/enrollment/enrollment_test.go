package enrollment_test

import (
	"context"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// --- Parse ------------------------------------------------------------------

func TestParse_Accepts_PAT(t *testing.T) {
	svc := newSvc(t)
	// Mint one to get a real-shaped string.
	admin, proj := bootstrap(t, svc.DB())
	res, err := svc.Svc.MintPAT(context.Background(), enrollment.MintPATInput{
		ProjectID: proj.ID, IssuedByUser: admin.ID, MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("MintPAT: %v", err)
	}

	parsed, err := enrollment.Parse(res.PlaintextToken)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Kind != enrollment.KindPAT {
		t.Fatalf("Kind = %v; want KindPAT", parsed.Kind)
	}
	if !strings.HasPrefix(parsed.ID, enrollment.PATPrefix) {
		t.Fatalf("ID %q missing plt_ prefix", parsed.ID)
	}
}

func TestParse_RejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"hello",
		"plt_onlyid",
		"plt_.onlysecret",
		"ghp_token",             // wrong prefix
		"plt_abcdef.NOT-BASE32", // dashes are not valid base32 chars
	}
	for _, c := range cases {
		if _, err := enrollment.Parse(c); err != enrollment.ErrMalformed {
			t.Errorf("Parse(%q) err = %v; want ErrMalformed", c, err)
		}
	}
}

// --- MintPAT + RedeemPAT happy path -----------------------------------------

func TestRedeemPAT_HappyPath_IssuesSession(t *testing.T) {
	svc := newSvc(t)
	admin, proj := bootstrap(t, svc.DB())
	ctx := context.Background()

	issue, err := svc.Svc.MintPAT(ctx, enrollment.MintPATInput{
		ProjectID: proj.ID, IssuedByUser: admin.ID, MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("MintPAT: %v", err)
	}

	res, err := svc.Svc.RedeemPAT(ctx, issue.PlaintextToken, enrollment.RedeemContext{
		ClientIP: "10.0.0.1", MachineID: "machine-1", Hostname: "webhost",
	})
	if err != nil {
		t.Fatalf("RedeemPAT: %v", err)
	}
	if res.Outcome != "success" {
		t.Fatalf("Outcome = %q; want success", res.Outcome)
	}
	if res.AgentID == "" || res.SessionPlaintext == "" {
		t.Fatalf("missing agent_id or session_token: %+v", res)
	}
	if !strings.HasPrefix(res.SessionPlaintext, enrollment.SessionPrefix) {
		t.Fatalf("SessionPlaintext missing sess_ prefix: %q", res.SessionPlaintext)
	}

	// PAT is now consumed; a second redemption must fail.
	again, err := svc.Svc.RedeemPAT(ctx, issue.PlaintextToken, enrollment.RedeemContext{})
	if err != nil {
		t.Fatalf("second RedeemPAT: %v", err)
	}
	if again.Outcome != "max_uses_reached" {
		t.Fatalf("second Outcome = %q; want max_uses_reached", again.Outcome)
	}

	// Audit: two redemption events — one success, one max_uses_reached.
	evts, err := svc.DB().PATRedemptionEvents().ListByToken(ctx, issue.TokenID, 10)
	if err != nil {
		t.Fatalf("ListByToken: %v", err)
	}
	if len(evts) != 2 {
		t.Fatalf("audit events = %d; want 2", len(evts))
	}
	outcomes := map[string]bool{}
	for _, e := range evts {
		outcomes[e.Outcome] = true
	}
	if !outcomes["success"] || !outcomes["max_uses_reached"] {
		t.Fatalf("outcomes = %+v; want success + max_uses_reached", outcomes)
	}
}

// --- RedeemSession + rotation -----------------------------------------------

func TestRedeemSession_RotatesAndInvalidatesOld(t *testing.T) {
	svc := newSvc(t)
	admin, proj := bootstrap(t, svc.DB())
	ctx := context.Background()

	// Enroll once to get an initial session.
	issue, err := svc.Svc.MintPAT(ctx, enrollment.MintPATInput{
		ProjectID: proj.ID, IssuedByUser: admin.ID, MaxUses: 1,
	})
	if err != nil {
		t.Fatalf("MintPAT: %v", err)
	}
	first, err := svc.Svc.RedeemPAT(ctx, issue.PlaintextToken, enrollment.RedeemContext{MachineID: "m1"})
	if err != nil || first.Outcome != "success" {
		t.Fatalf("enroll: %+v err=%v", first, err)
	}

	// Reconnect with the session token → rotation returns a new token.
	second, err := svc.Svc.RedeemSession(ctx, first.SessionPlaintext, enrollment.RedeemContext{MachineID: "m1"})
	if err != nil {
		t.Fatalf("RedeemSession: %v", err)
	}
	if second.Outcome != "success" {
		t.Fatalf("Outcome = %q; want success", second.Outcome)
	}
	if second.AgentID != first.AgentID {
		t.Fatalf("AgentID changed across rotation: %q -> %q", first.AgentID, second.AgentID)
	}
	if second.SessionID == first.SessionID {
		t.Fatal("SessionID did not change on rotation")
	}
	if second.SessionPlaintext == first.SessionPlaintext {
		t.Fatal("session plaintext did not change on rotation")
	}

	// Old token must be rejected now (session_inactive — it's rotated).
	replay, err := svc.Svc.RedeemSession(ctx, first.SessionPlaintext, enrollment.RedeemContext{})
	if err != nil {
		t.Fatalf("replay err: %v", err)
	}
	if replay.Outcome != "session_inactive" {
		t.Fatalf("replay Outcome = %q; want session_inactive", replay.Outcome)
	}
}

func TestRedeemSession_RejectsUnknown(t *testing.T) {
	svc := newSvc(t)
	res, err := svc.Svc.RedeemSession(context.Background(), "sess_aaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbbbbbb",
		enrollment.RedeemContext{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Outcome != "unknown_session" {
		t.Fatalf("Outcome = %q", res.Outcome)
	}
}

// --- RedeemPAT classifications ----------------------------------------------

func TestRedeemPAT_Malformed(t *testing.T) {
	svc := newSvc(t)
	res, err := svc.Svc.RedeemPAT(context.Background(), "garbage", enrollment.RedeemContext{})
	if err != enrollment.ErrMalformed {
		t.Fatalf("err = %v; want ErrMalformed", err)
	}
	if res.Outcome != "malformed" {
		t.Fatalf("Outcome = %q", res.Outcome)
	}
}

// --- helpers ---------------------------------------------------------------

type svcFixture struct {
	db  *storage.DB
	Svc *enrollment.Service
}

func (f *svcFixture) DB() *storage.DB { return f.db }

func newSvc(t *testing.T) *svcFixture {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &svcFixture{db: db, Svc: enrollment.New(db)}
}

func bootstrap(t *testing.T, db *storage.DB) (*user.User, *storage.Project) {
	t.Helper()
	ctx := context.Background()
	u := &user.User{
		ID:           "user-admin",
		Username:     "admin",
		PasswordHash: "hash",
		Role:         user.RoleAdmin,
	}
	if err := db.Users().Create(ctx, u); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	p := &storage.Project{
		ID:        "proj-1",
		Name:      "Project 1",
		Slug:      "p1",
		CreatedBy: u.ID,
	}
	if err := db.Projects().Create(ctx, p); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	return u, p
}
