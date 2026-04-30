package storage_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// SecurityScansRepo persists per-host hardening scan runs and their
// findings. Two normalised tables; severity counts are denormalised
// onto host_security_scans so the fleet view reads them in one
// query without joining N findings rows.

func seedHostForScans(t *testing.T, db *storage.DB, project *storage.Project, name string) *storage.Host {
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

func newScan(projectID, hostID string, started int64) *storage.SecurityScan {
	return &storage.SecurityScan{
		ID:            "scan-" + hostID + "-" + strconv.FormatInt(started, 10),
		ProjectID:     projectID,
		HostID:        hostID,
		StartedAtUnix: started,
		ElapsedMs:     42,
		ChecksJSON:    `[{"id":"ssh.config","status":"ok","elapsed_ms":3}]`,
	}
}

func newFinding(scanID, hostID, projectID, findingID, severity string) *storage.SecurityFinding {
	return &storage.SecurityFinding{
		ID:             "f-" + findingID + "-" + scanID,
		ScanID:         scanID,
		HostID:         hostID,
		ProjectID:      projectID,
		FindingID:      findingID,
		CheckID:        "ssh",
		Category:       "ssh",
		Severity:       severity,
		Title:          findingID + " title",
		Description:    "desc",
		Evidence:       "evidence for " + findingID,
		Remediation:    "fix it",
		ReferencesJSON: "[]",
	}
}

func TestSecurityScans_SaveAndLatest(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p", "Project", admin)
	host := seedHostForScans(t, db, proj, "alpha")

	scan := newScan(proj.ID, host.ID, time.Now().Unix())
	findings := []*storage.SecurityFinding{
		newFinding(scan.ID, host.ID, proj.ID, "ssh.permitrootlogin", storage.SeverityHigh),
		newFinding(scan.ID, host.ID, proj.ID, "ssh.passwordauthentication", storage.SeverityHigh),
		newFinding(scan.ID, host.ID, proj.ID, "kernel.kptr", storage.SeverityMedium),
	}
	if err := db.SecurityScans().Save(ctx, scan, findings); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, gotFindings, err := db.SecurityScans().LatestForHost(ctx, host.ID)
	if err != nil {
		t.Fatalf("LatestForHost: %v", err)
	}
	if got.ID != scan.ID {
		t.Fatalf("latest id = %q; want %q", got.ID, scan.ID)
	}
	// Severity counts must come back denormalised — proves Save
	// recomputed them from the findings list rather than trusting
	// caller-supplied zero values.
	if got.SeverityCounts.High != 2 || got.SeverityCounts.Medium != 1 {
		t.Fatalf("severity counts wrong: %+v", got.SeverityCounts)
	}
	if got.SeverityCounts.Critical != 0 || got.SeverityCounts.Low != 0 || got.SeverityCounts.Info != 0 {
		t.Fatalf("expected zeros for unused severities: %+v", got.SeverityCounts)
	}
	if len(gotFindings) != 3 {
		t.Fatalf("got %d findings; want 3", len(gotFindings))
	}
	// Findings come back ordered by severity rank then id.
	if gotFindings[0].Severity != "high" || gotFindings[2].Severity != "medium" {
		t.Fatalf("findings not severity-sorted: %+v", gotFindings)
	}
}

func TestSecurityScans_LatestForHost_NotFound(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p", "P", admin)
	host := seedHostForScans(t, db, proj, "neverScanned")
	_, _, err := db.SecurityScans().LatestForHost(ctx, host.ID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("LatestForHost = %v; want ErrNotFound", err)
	}
}

func TestSecurityScans_RetentionPrunesPast30(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p", "P", admin)
	host := seedHostForScans(t, db, proj, "alpha")

	// Insert 32 scans with monotonically-increasing timestamps. The
	// retention cap is 30 (SCAN_KEEP_PER_HOST), so the two oldest
	// scans should be pruned by the time the 32nd arrives.
	for i := 0; i < 32; i++ {
		scan := newScan(proj.ID, host.ID, int64(1_000_000+i))
		if err := db.SecurityScans().Save(ctx, scan, nil); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	scans, err := db.SecurityScans().ListScansForHost(ctx, host.ID, 50)
	if err != nil {
		t.Fatalf("ListScansForHost: %v", err)
	}
	if len(scans) != storage.SCAN_KEEP_PER_HOST {
		t.Fatalf("retention bug: got %d scans, want %d", len(scans), storage.SCAN_KEEP_PER_HOST)
	}
	// Oldest kept = 1_000_000 + 2 (we pruned timestamps 1_000_000 and 1_000_001).
	if scans[len(scans)-1].StartedAtUnix != 1_000_002 {
		t.Fatalf("oldest kept timestamp wrong: got %d, want 1_000_002", scans[len(scans)-1].StartedAtUnix)
	}
}

func TestSecurityScans_HostDeleteCascadesScansAndFindings(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p", "P", admin)
	host := seedHostForScans(t, db, proj, "alpha")

	scan := newScan(proj.ID, host.ID, 1)
	finds := []*storage.SecurityFinding{
		newFinding(scan.ID, host.ID, proj.ID, "f1", storage.SeverityHigh),
	}
	if err := db.SecurityScans().Save(ctx, scan, finds); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, "DELETE FROM hosts WHERE id = ?", host.ID); err != nil {
		t.Fatalf("delete host: %v", err)
	}

	var nScans, nFindings int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM host_security_scans WHERE host_id = ?", host.ID).Scan(&nScans); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM host_security_findings WHERE host_id = ?", host.ID).Scan(&nFindings); err != nil {
		t.Fatal(err)
	}
	if nScans != 0 || nFindings != 0 {
		t.Fatalf("cascade failed: scans=%d findings=%d", nScans, nFindings)
	}
}

func TestSecurityScans_LatestSummariesForProject(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p", "P", admin)
	other := seedProject(t, db, "p2", "Other", admin)

	hostA := seedHostForScans(t, db, proj, "a")
	hostB := seedHostForScans(t, db, proj, "b")
	hostNever := seedHostForScans(t, db, proj, "never")
	hostOther := seedHostForScans(t, db, other, "other")

	// Two scans on hostA — only the newer should appear in the summary.
	older := newScan(proj.ID, hostA.ID, 100)
	if err := db.SecurityScans().Save(ctx, older, []*storage.SecurityFinding{
		newFinding(older.ID, hostA.ID, proj.ID, "old.high", storage.SeverityHigh),
	}); err != nil {
		t.Fatal(err)
	}
	newer := newScan(proj.ID, hostA.ID, 200)
	if err := db.SecurityScans().Save(ctx, newer, []*storage.SecurityFinding{
		newFinding(newer.ID, hostA.ID, proj.ID, "new.crit", storage.SeverityCritical),
	}); err != nil {
		t.Fatal(err)
	}
	// hostB has one scan with no findings (clean) — must still
	// appear in the map so the UI can render the green "scanned,
	// clean" state vs the never-scanned state.
	bScan := newScan(proj.ID, hostB.ID, 150)
	if err := db.SecurityScans().Save(ctx, bScan, nil); err != nil {
		t.Fatal(err)
	}
	// hostOther is in a different project — must not leak into the
	// project=p summary.
	other1 := newScan(other.ID, hostOther.ID, 250)
	if err := db.SecurityScans().Save(ctx, other1, []*storage.SecurityFinding{
		newFinding(other1.ID, hostOther.ID, other.ID, "leak.crit", storage.SeverityCritical),
	}); err != nil {
		t.Fatal(err)
	}

	summaries, err := db.SecurityScans().LatestSummariesForProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("LatestSummariesForProject: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("want 2 summaries (hostA+hostB), got %d: %+v", len(summaries), summaries)
	}
	if _, ok := summaries[hostNever.ID]; ok {
		t.Fatalf("never-scanned host must be absent from summary map")
	}
	if _, ok := summaries[hostOther.ID]; ok {
		t.Fatalf("other-project host must not leak: %+v", summaries)
	}
	// hostA's summary points at the newer scan and reflects its
	// (Critical=1) histogram, NOT the older scan's (High=1).
	if a := summaries[hostA.ID]; a.ScanID != newer.ID || a.Counts.Critical != 1 || a.Counts.High != 0 {
		t.Fatalf("hostA summary wrong: %+v (want scan_id=%s, critical=1, high=0)", a, newer.ID)
	}
	// hostB scanned clean — present, all-zero counts.
	if b, ok := summaries[hostB.ID]; !ok {
		t.Fatalf("hostB missing from summary")
	} else if b.Counts != (storage.SeverityCounts{}) {
		t.Fatalf("hostB should be all-zero (scanned clean): %+v", b.Counts)
	}
}

func TestSecurityScans_ListFindings_FiltersAndPaging(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p", "P", admin)
	hostA := seedHostForScans(t, db, proj, "a")
	hostB := seedHostForScans(t, db, proj, "b")

	scanA := newScan(proj.ID, hostA.ID, 100)
	scanA.ID = "scan-a"
	if err := db.SecurityScans().Save(ctx, scanA, []*storage.SecurityFinding{
		findingWithCategory(scanA.ID, hostA.ID, proj.ID, "ssh.permitroot", storage.SeverityCritical, "ssh"),
		findingWithCategory(scanA.ID, hostA.ID, proj.ID, "kernel.kptr", storage.SeverityMedium, "kernel"),
		findingWithCategory(scanA.ID, hostA.ID, proj.ID, "kernel.bpf", storage.SeverityHigh, "kernel"),
	}); err != nil {
		t.Fatal(err)
	}
	scanB := newScan(proj.ID, hostB.ID, 200)
	scanB.ID = "scan-b"
	if err := db.SecurityScans().Save(ctx, scanB, []*storage.SecurityFinding{
		findingWithCategory(scanB.ID, hostB.ID, proj.ID, "ssh.passwordauth", storage.SeverityHigh, "ssh"),
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("severity filter", func(t *testing.T) {
		got, total, err := db.SecurityScans().ListFindings(ctx, proj.ID,
			storage.ListFindingsFilter{Severity: []string{storage.SeverityHigh}},
			storage.Page{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if total != 2 || len(got) != 2 {
			t.Fatalf("severity=high → total=%d items=%d", total, len(got))
		}
	})

	t.Run("category filter", func(t *testing.T) {
		_, total, err := db.SecurityScans().ListFindings(ctx, proj.ID,
			storage.ListFindingsFilter{Category: []string{"kernel"}},
			storage.Page{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if total != 2 {
			t.Fatalf("category=kernel total=%d; want 2", total)
		}
	})

	t.Run("host filter", func(t *testing.T) {
		got, _, err := db.SecurityScans().ListFindings(ctx, proj.ID,
			storage.ListFindingsFilter{HostID: hostB.ID},
			storage.Page{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].HostID != hostB.ID {
			t.Fatalf("host filter wrong: %+v", got)
		}
	})

	t.Run("substring search on title", func(t *testing.T) {
		got, _, err := db.SecurityScans().ListFindings(ctx, proj.ID,
			storage.ListFindingsFilter{Q: "permitroot"},
			storage.Page{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].FindingID != "ssh.permitroot" {
			t.Fatalf("title search wrong: %+v", got)
		}
	})

	t.Run("severity-rank ordering", func(t *testing.T) {
		got, _, err := db.SecurityScans().ListFindings(ctx, proj.ID,
			storage.ListFindingsFilter{}, storage.Page{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 4 {
			t.Fatalf("expected all 4 findings, got %d", len(got))
		}
		// Critical first, then high, then medium. Two Highs are
		// stable-ordered by id so the relative position of those two
		// is implementation-defined; we only assert the rank class.
		if got[0].Severity != storage.SeverityCritical {
			t.Fatalf("first severity = %q; want critical", got[0].Severity)
		}
		if got[len(got)-1].Severity != storage.SeverityMedium {
			t.Fatalf("last severity = %q; want medium", got[len(got)-1].Severity)
		}
	})

	t.Run("paging", func(t *testing.T) {
		// 4 findings; page_size=2 → 2 pages.
		first, total, err := db.SecurityScans().ListFindings(ctx, proj.ID,
			storage.ListFindingsFilter{}, storage.Page{Page: 1, PageSize: 2},
		)
		if err != nil || len(first) != 2 || total != 4 {
			t.Fatalf("page1: items=%d total=%d err=%v", len(first), total, err)
		}
		second, _, err := db.SecurityScans().ListFindings(ctx, proj.ID,
			storage.ListFindingsFilter{}, storage.Page{Page: 2, PageSize: 2},
		)
		if err != nil || len(second) != 2 {
			t.Fatalf("page2: items=%d err=%v", len(second), err)
		}
		if first[0].ID == second[0].ID {
			t.Fatalf("page 1 and 2 returned the same row %q", first[0].ID)
		}
	})
}

func TestSecurityScans_ListFindings_OnlyLatestScanPerHost(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p", "P", admin)
	host := seedHostForScans(t, db, proj, "alpha")

	older := newScan(proj.ID, host.ID, 100)
	older.ID = "old"
	if err := db.SecurityScans().Save(ctx, older, []*storage.SecurityFinding{
		newFinding(older.ID, host.ID, proj.ID, "old.crit", storage.SeverityCritical),
	}); err != nil {
		t.Fatal(err)
	}
	newer := newScan(proj.ID, host.ID, 200)
	newer.ID = "new"
	if err := db.SecurityScans().Save(ctx, newer, []*storage.SecurityFinding{
		newFinding(newer.ID, host.ID, proj.ID, "new.high", storage.SeverityHigh),
	}); err != nil {
		t.Fatal(err)
	}

	got, total, err := db.SecurityScans().ListFindings(ctx, proj.ID,
		storage.ListFindingsFilter{}, storage.Page{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(got) != 1 {
		t.Fatalf("project view should restrict to latest scan per host: total=%d items=%d", total, len(got))
	}
	if got[0].FindingID != "new.high" {
		t.Fatalf("returned the wrong scan's finding: %+v", got[0])
	}
}

func findingWithCategory(scanID, hostID, projectID, findingID, severity, category string) *storage.SecurityFinding {
	f := newFinding(scanID, hostID, projectID, findingID, severity)
	f.Category = category
	f.CheckID = category
	return f
}
