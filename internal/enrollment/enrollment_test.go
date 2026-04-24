package enrollment_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/activity"
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
		"sess_abc.def",          // session tokens no longer parsed (post-v2)
		"plt_abcdef.NOT-BASE32", // dashes are not valid base32 chars
	}
	for _, c := range cases {
		if _, err := enrollment.Parse(c); err != enrollment.ErrMalformed {
			t.Errorf("Parse(%q) err = %v; want ErrMalformed", c, err)
		}
	}
}

// --- MintPAT + RedeemPAT happy path -----------------------------------------

func TestRedeemPAT_HappyPath(t *testing.T) {
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
	if res.AgentID == "" {
		t.Fatalf("missing agent_id: %+v", res)
	}
	if !strings.HasPrefix(res.AgentID, "agent-") {
		t.Fatalf("AgentID missing agent- prefix: %q", res.AgentID)
	}
	if res.ProjectID != proj.ID {
		t.Fatalf("ProjectID = %q; want %q", res.ProjectID, proj.ID)
	}

	// PAT is now consumed; a second redemption must fail.
	again, err := svc.Svc.RedeemPAT(ctx, issue.PlaintextToken, enrollment.RedeemContext{})
	if err != nil {
		t.Fatalf("second RedeemPAT: %v", err)
	}
	if again.Outcome != "max_uses_reached" {
		t.Fatalf("second Outcome = %q; want max_uses_reached", again.Outcome)
	}

	// Audit: two redemption rows in the unified activities log — one
	// success, one rejection with reason "max_uses_reached".
	// The recorder writes asynchronously, so give it a moment.
	waitForActivities(t, svc.DB(), storage.ActivityFilter{
		TargetType: "pat_token",
		TargetID:   issue.TokenID,
		Limit:      10,
	}, 2)

	evts, _, err := svc.DB().Activities().List(ctx, storage.ActivityFilter{
		TargetType: "pat_token",
		TargetID:   issue.TokenID,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("Activities().List: %v", err)
	}
	if len(evts) != 2 {
		t.Fatalf("activity events = %d; want 2", len(evts))
	}
	// Outcome is success or denied; reason lives in meta.
	outcomes := map[string]bool{}
	for _, e := range evts {
		outcomes[e.Outcome] = true
	}
	if !outcomes[storage.OutcomeSuccess] || !outcomes[storage.OutcomeDenied] {
		t.Fatalf("outcomes = %+v; want success + denied", outcomes)
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
	// Install a per-test activity recorder so enrollment audit writes
	// hit this DB. Each test owns its own singleton slot; the tests
	// don't run concurrently in this package, so the race-free
	// atomic-pointer set is fine.
	activity.SetRecorder(activity.New(db))
	t.Cleanup(func() { activity.SetRecorder(nil) })
	return &svcFixture{db: db, Svc: enrollment.New(db)}
}

// waitForActivities polls the activities table until at least want rows
// match the filter, or the deadline passes. Needed because the recorder
// writes asynchronously.
func waitForActivities(t *testing.T, db *storage.DB, f storage.ActivityFilter, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, _, err := db.Activities().List(context.Background(), f)
		if err != nil {
			t.Fatalf("Activities().List: %v", err)
		}
		if len(got) >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d activities; got %d", want, len(got))
		}
		time.Sleep(10 * time.Millisecond)
	}
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
