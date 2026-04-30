package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Per-host security scan endpoints. The POST .../security-scan path
// requires a live agent and is exercised via the agent_link
// integration tests; the unit tests here cover the GET endpoints,
// the proto→storage adapter, and the hosts-list enrichment.

func seedHostForSecurityTest(t *testing.T, db *storage.DB, project *storage.Project, name string) *storage.Host {
	t.Helper()
	host, err := db.Hosts().Upsert(context.Background(), &storage.HostIdentity{
		ProjectID:   project.ID,
		MachineID:   "m-" + name,
		Fingerprint: "fp-" + name,
		Hostname:    name,
		OS:          "linux",
		SeenAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed host %s: %v", name, err)
	}
	return host
}

func saveScan(t *testing.T, db *storage.DB, projectID, hostID string, started int64, sevs ...string) *storage.SecurityScan {
	t.Helper()
	scanID := "scan-" + hostID + "-" + strconv.FormatInt(started, 10)
	scan := &storage.SecurityScan{
		ID:            scanID,
		ProjectID:     projectID,
		HostID:        hostID,
		StartedAtUnix: started,
		ElapsedMs:     100,
		ChecksJSON:    `[]`,
	}
	findings := make([]*storage.SecurityFinding, 0, len(sevs))
	for i, sev := range sevs {
		findings = append(findings, &storage.SecurityFinding{
			ID:             "f-" + scanID + "-" + strconv.Itoa(i),
			ScanID:         scanID,
			HostID:         hostID,
			ProjectID:      projectID,
			FindingID:      "test." + sev,
			CheckID:        "test",
			Category:       "ssh",
			Severity:       sev,
			Title:          sev + " finding",
			Description:    "desc",
			Evidence:       "ev",
			Remediation:    "fix",
			ReferencesJSON: "[]",
		})
	}
	if err := db.SecurityScans().Save(context.Background(), scan, findings); err != nil {
		t.Fatalf("Save scan: %v", err)
	}
	return scan
}

func TestSecurity_GetLatest_404WhenNeverScanned(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p", admin)
	host := seedHostForSecurityTest(t, db, proj, "alpha")

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/hosts/"+host.ID+"/security-scan", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s; want 404", w.Code, w.Body.String())
	}
}

func TestSecurity_GetLatest_ReturnsPersistedScan(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p", admin)
	host := seedHostForSecurityTest(t, db, proj, "alpha")
	saveScan(t, db, proj.ID, host.ID, 100, storage.SeverityHigh, storage.SeverityMedium)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/hosts/"+host.ID+"/security-scan", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp scanResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SeverityCounts.High != 1 || resp.SeverityCounts.Medium != 1 {
		t.Fatalf("severity counts: %+v", resp.SeverityCounts)
	}
	if len(resp.Findings) != 2 {
		t.Fatalf("findings count: got %d; want 2", len(resp.Findings))
	}
}

func TestSecurity_GetLatest_CrossProjectIsolated(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	prod := seedProjectForAPITest(t, db, "prod", admin)
	stag := seedProjectForAPITest(t, db, "stag", admin)
	host := seedHostForSecurityTest(t, db, prod, "alpha")
	saveScan(t, db, prod.ID, host.ID, 100, storage.SeverityHigh)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	// Prod host id under staging URL must 404.
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+stag.ID+"/hosts/"+host.ID+"/security-scan", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-project leak: status=%d", w.Code)
	}
}

func TestSecurity_GetByScanID_RejectsForeignScan(t *testing.T) {
	// A scan id from one host must not be retrievable via another
	// host's URL — even within the same project, even when both
	// hosts have been scanned. Defensive: catches a UI bug that
	// pastes the wrong id, and forecloses any future endpoint that
	// might leak ids across hosts.
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p", admin)
	hostA := seedHostForSecurityTest(t, db, proj, "a")
	hostB := seedHostForSecurityTest(t, db, proj, "b")
	scanA := saveScan(t, db, proj.ID, hostA.ID, 100, storage.SeverityHigh)
	saveScan(t, db, proj.ID, hostB.ID, 200)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	// hostB URL with hostA's scan_id → 404, not 200.
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/hosts/"+hostB.ID+"/security-scan?scan_id="+scanA.ID, tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("foreign scan id leaked: status=%d", w.Code)
	}
}

func TestSecurity_ListScansForHost(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p", admin)
	host := seedHostForSecurityTest(t, db, proj, "alpha")
	saveScan(t, db, proj.ID, host.ID, 100)
	saveScan(t, db, proj.ID, host.ID, 200, storage.SeverityHigh)
	saveScan(t, db, proj.ID, host.ID, 300, storage.SeverityCritical)

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET",
		"/api/v1/projects/"+proj.ID+"/hosts/"+host.ID+"/security-scans", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Scans []scanSummaryResponse `json:"scans"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Scans) != 3 {
		t.Fatalf("got %d scans; want 3", len(resp.Scans))
	}
	if resp.Scans[0].StartedAtUnix != 300 || resp.Scans[2].StartedAtUnix != 100 {
		t.Fatalf("scans not newest-first: %+v", resp.Scans)
	}
	if resp.Scans[0].SeverityCounts.Critical != 1 {
		t.Fatalf("severity counts dropped: %+v", resp.Scans[0])
	}
}

func TestHosts_List_EnrichesWithScanSummary(t *testing.T) {
	r, db := hostsTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p", admin)
	scanned := seedHostForSecurityTest(t, db, proj, "scanned")
	clean := seedHostForSecurityTest(t, db, proj, "clean")
	_ = seedHostForSecurityTest(t, db, proj, "never") // exists, never scanned
	saveScan(t, db, proj.ID, scanned.ID, 100, storage.SeverityHigh, storage.SeverityHigh, storage.SeverityMedium)
	saveScan(t, db, proj.ID, clean.ID, 150) // scanned but no findings

	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)
	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/hosts", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Hosts []hostResponse `json:"hosts"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	got := map[string]hostResponse{}
	for _, h := range resp.Hosts {
		got[h.Hostname] = h
	}
	// Scanned host: counts populated, non-zero histogram.
	if h := got["scanned"]; h.SecuritySeverityCounts == nil || h.SecuritySeverityCounts.High != 2 {
		t.Fatalf("scanned host missing or wrong counts: %+v", h.SecuritySeverityCounts)
	}
	// Cleanly-scanned host: counts populated but zero across the
	// board (UI distinguishes "scanned & clean" from "never scanned").
	if h := got["clean"]; h.SecuritySeverityCounts == nil ||
		(*h.SecuritySeverityCounts) != (storage.SeverityCounts{}) {
		t.Fatalf("clean host: expected zero counts present, got %+v", h.SecuritySeverityCounts)
	}
	// Never-scanned host: counts must be nil so the UI knows to
	// hide the badge entirely.
	if h := got["never"]; h.SecuritySeverityCounts != nil {
		t.Fatalf("never-scanned host should have nil counts, got %+v", h.SecuritySeverityCounts)
	}
}

// TestBuildStorageRows pins the proto→storage adapter so the UI's
// finding/check shape can't drift from the agent's wire format.
func TestBuildStorageRows(t *testing.T) {
	src := &v2pb.SecurityScanResponse{
		StartedAtUnix: 12345,
		ElapsedMs:     7,
		Findings: []*v2pb.SecurityFinding{
			{
				Id: "ssh.permitrootlogin", CheckId: "ssh", Category: "ssh",
				Severity: "high", Title: "Root login", Description: "d",
				Evidence: "e", Remediation: "r",
				References: []string{"CVE-2021-0000"},
			},
		},
		Checks: []*v2pb.CheckResult{
			{Id: "ssh.config", Category: "ssh", Status: "ok", ElapsedMs: 3, FindingCount: 1},
		},
	}
	scan, findings := buildStorageRows("p1", "h1", src)
	if scan.StartedAtUnix != 12345 || scan.ElapsedMs != 7 || scan.HostID != "h1" || scan.ProjectID != "p1" {
		t.Fatalf("scan adapter wrong: %+v", scan)
	}
	if scan.ChecksJSON == "" || scan.ChecksJSON == "[]" {
		t.Fatalf("checks_json empty; want serialised array")
	}
	if len(findings) != 1 {
		t.Fatalf("findings count: got %d", len(findings))
	}
	f := findings[0]
	if f.FindingID != "ssh.permitrootlogin" || f.Severity != "high" {
		t.Fatalf("finding adapter wrong: %+v", f)
	}
	if f.ReferencesJSON != `["CVE-2021-0000"]` {
		t.Fatalf("references_json wrong: %q", f.ReferencesJSON)
	}
	// Each row gets a fresh id so two concurrent scans on the same
	// host can't collide on the primary key.
	if f.ID == "" || scan.ID == "" {
		t.Fatalf("ids not generated")
	}
}
