package api

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

func auditExportTestSetup(t *testing.T) (*gin.Engine, *storage.DB, *TokenIssuer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	issuer, _ := NewTokenIssuer("a", "b", 5*time.Minute, time.Hour)
	rbac := NewRBACWithStorage(issuer, db)
	h := NewAuditHandler(db)

	r := gin.New()
	RegisterV1AuditRoutes(r, h, rbac)
	return r, db, issuer
}

// Seed a fixture covering every event family scoped to the project we
// want to export, plus one out-of-scope row per family to prove the
// filter is working.
func seedAuditFixture(t *testing.T, db *storage.DB, proj, otherProj *storage.Project, admin *user.User) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	// In-scope PAT + events.
	patIn := &storage.PATToken{
		TokenID:      "plt_in",
		SecretHash:   hashBytesAPI([]byte("x")),
		ProjectID:    proj.ID,
		IssuedByUser: admin.ID,
		IssuedAt:     now,
		ExpiresAt:    now.Add(time.Hour),
		MaxUses:      1,
	}
	if err := db.PATTokens().Create(ctx, patIn); err != nil {
		t.Fatalf("seed PAT in: %v", err)
	}
	patOut := &storage.PATToken{
		TokenID:      "plt_out",
		SecretHash:   hashBytesAPI([]byte("x")),
		ProjectID:    otherProj.ID,
		IssuedByUser: admin.ID,
		IssuedAt:     now,
		ExpiresAt:    now.Add(time.Hour),
		MaxUses:      1,
	}
	if err := db.PATTokens().Create(ctx, patOut); err != nil {
		t.Fatalf("seed PAT out: %v", err)
	}

	_ = db.PATRedemptionEvents().Record(ctx, &storage.PATRedemptionEvent{
		At: now, TokenID: "plt_in", Outcome: "success",
	})
	_ = db.PATRedemptionEvents().Record(ctx, &storage.PATRedemptionEvent{
		At: now, TokenID: "plt_out", Outcome: "success",
	})

	// Connection events — scoped by agent_sessions.project_id.
	inSess := &storage.AgentSession{
		SessionID:        "sess-in",
		AgentID:          "agent-in",
		ProjectID:        proj.ID,
		SessionTokenHash: []byte("h"),
		IssuedAt:         now,
		IssuedReason:     "enroll",
		ExpiresAt:        now.Add(time.Hour),
	}
	_ = db.AgentSessions().InsertActive(ctx, inSess)
	outSess := &storage.AgentSession{
		SessionID:        "sess-out",
		AgentID:          "agent-out",
		ProjectID:        otherProj.ID,
		SessionTokenHash: []byte("h"),
		IssuedAt:         now,
		IssuedReason:     "enroll",
		ExpiresAt:        now.Add(time.Hour),
	}
	_ = db.AgentSessions().InsertActive(ctx, outSess)

	_ = db.AgentConnectionEvents().Record(ctx, &storage.AgentConnectionEvent{
		At: now, AgentID: "agent-in", EventType: "enroll_success", Transport: "tls_direct",
	})
	_ = db.AgentConnectionEvents().Record(ctx, &storage.AgentConnectionEvent{
		At: now, AgentID: "agent-out", EventType: "enroll_success", Transport: "tls_direct",
	})

	// Admin actions.
	_ = db.AdminAuditLog().Record(ctx, &storage.AdminAuditEvent{
		At: now, ActorUser: admin.ID, Action: "pat.issue", ProjectID: proj.ID, Outcome: "success",
	})
	_ = db.AdminAuditLog().Record(ctx, &storage.AdminAuditEvent{
		At: now, ActorUser: admin.ID, Action: "pat.issue", ProjectID: otherProj.ID, Outcome: "success",
	})
}

// NDJSON export returns all three kinds filtered to the project, plus
// the meta-audit row our own call wrote (admin kind). That meta-audit
// shows up BEFORE the stream since it's inserted by Export before the
// stream writes — confirming the "export is itself audited" contract.
func TestAuditExport_JSONL_ProjectScoped(t *testing.T) {
	r, db, issuer := auditExportTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p1", admin)
	other := seedProjectForAPITest(t, db, "p2", admin)
	seedAuditFixture(t, db, proj, other, admin)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/audit/export?format=jsonl", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/x-ndjson") {
		t.Fatalf("Content-Type = %q", ct)
	}

	// Parse each NDJSON line and tally per kind. Out-of-scope events
	// must be absent; each in-scope family must appear at least once.
	counts := map[string]int{}
	scan := bufio.NewScanner(strings.NewReader(w.Body.String()))
	for scan.Scan() {
		line := scan.Bytes()
		if len(line) == 0 {
			continue
		}
		var envelope struct {
			Kind  string          `json:"kind"`
			Event json.RawMessage `json:"event"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			t.Fatalf("bad line %q: %v", line, err)
		}
		counts[envelope.Kind]++

		// Spot-check that an in-scope event never contains the other
		// project's ids. Cheap plain-text sweep.
		if strings.Contains(string(envelope.Event), other.ID) {
			t.Errorf("line %q leaked project %s", envelope.Event, other.ID)
		}
		if strings.Contains(string(envelope.Event), "plt_out") ||
			strings.Contains(string(envelope.Event), "agent-out") {
			t.Errorf("out-of-scope event leaked: %q", envelope.Event)
		}
	}
	if counts["pat_redemption"] < 1 {
		t.Errorf("no pat_redemption events in export")
	}
	if counts["connection"] < 1 {
		t.Errorf("no connection events in export")
	}
	if counts["admin"] < 1 {
		t.Errorf("no admin events in export")
	}

	// Meta-audit: the "audit.export" row must be present in
	// admin_audit_log after the call.
	all, err := db.AdminAuditLog().ListRecent(testCtx(), 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	var sawMeta bool
	for _, e := range all {
		if e.Action == "audit.export" && e.TargetID == proj.ID {
			sawMeta = true
			break
		}
	}
	if !sawMeta {
		t.Fatal("meta-audit row missing for audit.export")
	}
}

func TestAuditExport_CSV_HeadersAndKindColumn(t *testing.T) {
	r, db, issuer := auditExportTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p1", admin)
	other := seedProjectForAPITest(t, db, "p2", admin)
	seedAuditFixture(t, db, proj, other, admin)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/audit/export?format=csv&types=pat_redemption,admin",
		tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if disp := w.Header().Get("Content-Disposition"); !strings.Contains(disp, "attachment") {
		t.Fatalf("Content-Disposition = %q", disp)
	}

	// Parse the CSV in streams separated by blank lines; every stream's
	// first row is a header that starts with "kind".
	reader := csv.NewReader(strings.NewReader(w.Body.String()))
	reader.FieldsPerRecord = -1 // streams have different widths
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v", err)
	}
	headerKinds := map[string]bool{}
	for _, row := range records {
		if len(row) > 0 && row[0] == "kind" {
			// header row — next row starts this kind's data
			continue
		}
		if len(row) > 0 && row[0] != "" {
			headerKinds[row[0]] = true
		}
	}
	// We requested pat_redemption + admin; both should appear, others shouldn't.
	if !headerKinds["pat_redemption"] {
		t.Errorf("pat_redemption missing from CSV")
	}
	if !headerKinds["admin"] {
		t.Errorf("admin missing from CSV")
	}
	if headerKinds["connection"] {
		t.Errorf("connection leaked into CSV despite not being requested")
	}
}

func TestAuditExport_BadFormatRejected(t *testing.T) {
	r, db, issuer := auditExportTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p1", admin)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/audit/export?format=xml", tok, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d; want 400", w.Code)
	}
}

func TestAuditExport_FromParseError(t *testing.T) {
	r, db, issuer := auditExportTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p1", admin)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/audit/export?from=notanumber", tok, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

// Canary: make sure we don't accidentally break when a kind is empty.
func TestAuditExport_NoMatchingEvents(t *testing.T) {
	r, db, issuer := auditExportTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p1", admin)

	tok, _ := issuer.IssueAccess(AccessClaims{UserID: admin.ID, Role: user.RoleAdmin})
	w := probeReqWithPath(r, "GET",
		fmt.Sprintf("/api/v1/projects/%s/audit/export?format=jsonl", proj.ID),
		tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	// Body may still contain the meta-audit event (added for this call);
	// that's fine, we just check the call didn't error.
	_ = w.Body.String()
}

// Small helper mirroring storage_test.hashBytes — we can't import that
// file across packages, and the audit tests don't need real hashes.
func hashBytesAPI(b []byte) []byte {
	out := make([]byte, 32)
	copy(out, b)
	return out
}
