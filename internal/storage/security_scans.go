package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Severity values for SecurityFinding.Severity. The agent emits the
// same lowercase tokens; the server stores them verbatim and the UI
// renders against this fixed ladder.
const (
	SeverityInfo     = "info"
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// SCAN_KEEP_PER_HOST caps the per-host scan history. Older scans are
// pruned in the same transaction that inserts a new one so the
// table never grows unbounded. The findings table cascades through
// the scans FK so it never out-grows the parent.
const SCAN_KEEP_PER_HOST = 30

// SecurityScan is one row in host_security_scans. The denormalised
// finding-count fields mirror the actual rows in
// host_security_findings; both Save and the UI rely on them so the
// fleet/host-list endpoint can render severity badges without
// joining N findings.
type SecurityScan struct {
	ID              string
	ProjectID       string
	HostID          string
	StartedAtUnix   int64
	ElapsedMs       int64
	Error           string
	SeverityCounts  SeverityCounts
	ChecksJSON      string // raw JSON; opaque to the storage layer
}

// SecurityFinding is one row in host_security_findings.
// ReferencesJSON stores the JSON-encoded references slice as it
// arrived from the agent; callers convert to/from []string as
// needed.
type SecurityFinding struct {
	ID             string
	ScanID         string
	HostID         string
	ProjectID      string
	FindingID      string
	CheckID        string
	Category       string
	Severity       string
	Title          string
	Description    string
	Evidence       string
	Remediation    string
	ReferencesJSON string
	ScannedAtUnix  int64
}

// SeverityCounts is a per-severity histogram used both as a row
// payload (denormalised on host_security_scans) and as a summary
// type returned to the UI.
type SeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
}

// ScanSummary is the per-host snapshot the hosts list endpoint
// returns. ScannedAt is the wall clock of the most recent scan;
// Counts is its denormalised severity histogram. Hosts that have
// never been scanned simply don't appear in the map (NOT a zero-
// value summary — "never scanned" must be UI-distinct from "scanned
// and clean").
type ScanSummary struct {
	ScanID        string         `json:"scan_id"`
	StartedAtUnix int64          `json:"started_at_unix"`
	Counts        SeverityCounts `json:"counts"`
}

// ListFindingsFilter is the search shape for the project-level
// Security page. Empty slices = no filter on that axis. HostID = ""
// = all hosts. Q is a case-insensitive substring match on title
// and evidence; empty = no text filter.
type ListFindingsFilter struct {
	Severity []string
	Category []string
	HostID   string
	Q        string
}

// Page narrows the result to one page of size PageSize, 1-indexed.
// Page=0 is treated as 1; PageSize=0 falls back to 50.
type Page struct {
	Page     int
	PageSize int
}

func (p Page) effective() (page, size, offset int) {
	page = p.Page
	if page < 1 {
		page = 1
	}
	size = p.PageSize
	if size <= 0 {
		size = 50
	}
	if size > 200 {
		size = 200
	}
	return page, size, (page - 1) * size
}

// SecurityScansRepo owns reads and writes against host_security_scans
// + host_security_findings.
type SecurityScansRepo struct {
	db *sql.DB
}

func (db *DB) SecurityScans() *SecurityScansRepo {
	return &SecurityScansRepo{db: db.DB}
}

// Save inserts one scan and its findings in a single transaction.
// The denormalised severity counters on SecurityScan are recomputed
// from the supplied findings rather than trusted from the caller —
// that way the scan row and the findings table can never disagree.
//
// As part of the same transaction, scans for HostID older than
// SCAN_KEEP_PER_HOST are deleted. The CASCADE on host_security_findings
// removes their findings.
func (r *SecurityScansRepo) Save(ctx context.Context, scan *SecurityScan, findings []*SecurityFinding) error {
	if scan == nil {
		return fmt.Errorf("security_scans: Save with nil scan")
	}
	scan.SeverityCounts = countSeverities(findings)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("security_scans: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO host_security_scans (
			id, project_id, host_id, started_at_unix, elapsed_ms, error,
			finding_count_critical, finding_count_high, finding_count_medium,
			finding_count_low, finding_count_info, checks_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		scan.ID, scan.ProjectID, scan.HostID, scan.StartedAtUnix, scan.ElapsedMs, scan.Error,
		scan.SeverityCounts.Critical, scan.SeverityCounts.High, scan.SeverityCounts.Medium,
		scan.SeverityCounts.Low, scan.SeverityCounts.Info, scan.ChecksJSON,
	); err != nil {
		return fmt.Errorf("security_scans: insert scan: %w", err)
	}

	for _, f := range findings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO host_security_findings (
				id, scan_id, host_id, project_id, finding_id, check_id, category,
				severity, title, description, evidence, remediation,
				references_json, scanned_at_unix
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ID, scan.ID, scan.HostID, scan.ProjectID, f.FindingID, f.CheckID, f.Category,
			f.Severity, f.Title, f.Description, f.Evidence, f.Remediation,
			f.ReferencesJSON, scan.StartedAtUnix,
		); err != nil {
			return fmt.Errorf("security_scans: insert finding %s: %w", f.FindingID, err)
		}
	}

	// Retention: keep the most recent SCAN_KEEP_PER_HOST scans and
	// delete everything older. The CASCADE on findings.scan_id wipes
	// their findings. Subquery uses the (host_id, started_at_unix
	// DESC) index added by migration 000024.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM host_security_scans
		 WHERE host_id = ?
		   AND id NOT IN (
		       SELECT id FROM host_security_scans
		        WHERE host_id = ?
		        ORDER BY started_at_unix DESC
		        LIMIT ?
		   )`,
		scan.HostID, scan.HostID, SCAN_KEEP_PER_HOST,
	); err != nil {
		return fmt.Errorf("security_scans: prune retention: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("security_scans: commit: %w", err)
	}
	return nil
}

// LatestForHost returns the most recent scan + its findings for
// hostID. Returns ErrNotFound when the host has never been scanned;
// the API layer maps that to a 404 (UI distinguishes never-scanned
// from scanned-clean).
func (r *SecurityScansRepo) LatestForHost(ctx context.Context, hostID string) (*SecurityScan, []*SecurityFinding, error) {
	scan, err := r.latestScanRow(ctx, hostID)
	if err != nil {
		return nil, nil, err
	}
	findings, err := r.findingsForScan(ctx, scan.ID)
	if err != nil {
		return nil, nil, err
	}
	return scan, findings, nil
}

// GetScan returns a specific scan + findings by id (used by the per-
// host History dropdown). Returns ErrNotFound when the scan id is
// unknown.
func (r *SecurityScansRepo) GetScan(ctx context.Context, scanID string) (*SecurityScan, []*SecurityFinding, error) {
	row := r.db.QueryRowContext(ctx, scanSelectColumns+` FROM host_security_scans WHERE id = ?`, scanID)
	scan, err := scanScanRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	findings, err := r.findingsForScan(ctx, scanID)
	if err != nil {
		return nil, nil, err
	}
	return scan, findings, nil
}

// ListScansForHost returns the lightweight scan rows (no findings)
// newest-first, capped at limit (default 10, max 50). Used by the
// per-host History dropdown.
func (r *SecurityScansRepo) ListScansForHost(ctx context.Context, hostID string, limit int) ([]*SecurityScan, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		scanSelectColumns+` FROM host_security_scans WHERE host_id = ? ORDER BY started_at_unix DESC LIMIT ?`,
		hostID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*SecurityScan
	for rows.Next() {
		s, err := scanScanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// LatestSummariesForProject returns one entry per host in projectID,
// keyed by host_id, populated from each host's most recent scan.
// Hosts with no scans are absent from the map (callers that need a
// dense map per host id supply their own zero value).
func (r *SecurityScansRepo) LatestSummariesForProject(ctx context.Context, projectID string) (map[string]ScanSummary, error) {
	// Pick the latest scan per host via a correlated MAX subquery.
	// The (host_id, started_at_unix DESC) index keeps this O(log n)
	// per host.
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.host_id, s.id, s.started_at_unix,
		       s.finding_count_critical, s.finding_count_high,
		       s.finding_count_medium, s.finding_count_low, s.finding_count_info
		  FROM host_security_scans s
		 WHERE s.project_id = ?
		   AND s.started_at_unix = (
		       SELECT MAX(started_at_unix) FROM host_security_scans
		        WHERE host_id = s.host_id
		   )`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]ScanSummary{}
	for rows.Next() {
		var (
			hostID  string
			summary ScanSummary
		)
		if err := rows.Scan(
			&hostID, &summary.ScanID, &summary.StartedAtUnix,
			&summary.Counts.Critical, &summary.Counts.High,
			&summary.Counts.Medium, &summary.Counts.Low, &summary.Counts.Info,
		); err != nil {
			return nil, err
		}
		out[hostID] = summary
	}
	return out, rows.Err()
}

// ListFindings powers the project-level Security page. Restricted
// to the latest scan per host on the server side so the fleet view
// always shows current posture, never historical noise. Returns the
// findings page plus the total count for pagination.
func (r *SecurityScansRepo) ListFindings(ctx context.Context, projectID string, filter ListFindingsFilter, page Page) ([]*SecurityFinding, int, error) {
	conds := []string{"f.project_id = ?"}
	args := []any{projectID}

	// Scope to latest scan per host — the same correlated MAX shape
	// LatestSummariesForProject uses.
	conds = append(conds, `f.scan_id = (
		SELECT id FROM host_security_scans s
		 WHERE s.host_id = f.host_id
		 ORDER BY s.started_at_unix DESC
		 LIMIT 1
	)`)

	if len(filter.Severity) > 0 {
		conds = append(conds, "f.severity IN ("+placeholders(len(filter.Severity))+")")
		for _, s := range filter.Severity {
			args = append(args, s)
		}
	}
	if len(filter.Category) > 0 {
		conds = append(conds, "f.category IN ("+placeholders(len(filter.Category))+")")
		for _, c := range filter.Category {
			args = append(args, c)
		}
	}
	if filter.HostID != "" {
		conds = append(conds, "f.host_id = ?")
		args = append(args, filter.HostID)
	}
	if q := strings.TrimSpace(filter.Q); q != "" {
		conds = append(conds, "(LOWER(f.title) LIKE ? OR LOWER(f.evidence) LIKE ?)")
		needle := "%" + strings.ToLower(q) + "%"
		args = append(args, needle, needle)
	}

	where := strings.Join(conds, " AND ")

	// Count first — bounded query that doesn't move pages around as
	// the caller paginates. Uses the same WHERE so filters apply.
	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM host_security_findings f WHERE `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	_, size, offset := page.effective()
	args = append(args, size, offset)
	rows, err := r.db.QueryContext(ctx,
		findingSelectColumns+` FROM host_security_findings f
		 WHERE `+where+`
		 ORDER BY CASE f.severity
		     WHEN 'critical' THEN 0
		     WHEN 'high'     THEN 1
		     WHEN 'medium'   THEN 2
		     WHEN 'low'      THEN 3
		     WHEN 'info'     THEN 4
		     ELSE 5
		 END,
		 f.scanned_at_unix DESC,
		 f.id ASC
		 LIMIT ? OFFSET ?`, args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	var out []*SecurityFinding
	for rows.Next() {
		f, err := scanFindingRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, f)
	}
	return out, total, rows.Err()
}

// --- internals ----------------------------------------------------

const scanSelectColumns = `SELECT
	id, project_id, host_id, started_at_unix, elapsed_ms, error,
	finding_count_critical, finding_count_high, finding_count_medium,
	finding_count_low, finding_count_info, checks_json`

const findingSelectColumns = `SELECT
	f.id, f.scan_id, f.host_id, f.project_id, f.finding_id, f.check_id,
	f.category, f.severity, f.title, f.description, f.evidence,
	f.remediation, f.references_json, f.scanned_at_unix`

func scanScanRow(row interface{ Scan(...any) error }) (*SecurityScan, error) {
	var s SecurityScan
	if err := row.Scan(
		&s.ID, &s.ProjectID, &s.HostID, &s.StartedAtUnix, &s.ElapsedMs, &s.Error,
		&s.SeverityCounts.Critical, &s.SeverityCounts.High, &s.SeverityCounts.Medium,
		&s.SeverityCounts.Low, &s.SeverityCounts.Info, &s.ChecksJSON,
	); err != nil {
		return nil, err
	}
	return &s, nil
}

func scanFindingRow(row interface{ Scan(...any) error }) (*SecurityFinding, error) {
	var f SecurityFinding
	if err := row.Scan(
		&f.ID, &f.ScanID, &f.HostID, &f.ProjectID, &f.FindingID, &f.CheckID,
		&f.Category, &f.Severity, &f.Title, &f.Description, &f.Evidence,
		&f.Remediation, &f.ReferencesJSON, &f.ScannedAtUnix,
	); err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *SecurityScansRepo) latestScanRow(ctx context.Context, hostID string) (*SecurityScan, error) {
	row := r.db.QueryRowContext(ctx,
		scanSelectColumns+` FROM host_security_scans WHERE host_id = ? ORDER BY started_at_unix DESC LIMIT 1`,
		hostID,
	)
	s, err := scanScanRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *SecurityScansRepo) findingsForScan(ctx context.Context, scanID string) ([]*SecurityFinding, error) {
	rows, err := r.db.QueryContext(ctx,
		findingSelectColumns+` FROM host_security_findings f WHERE f.scan_id = ?
		 ORDER BY CASE f.severity
		     WHEN 'critical' THEN 0
		     WHEN 'high'     THEN 1
		     WHEN 'medium'   THEN 2
		     WHEN 'low'      THEN 3
		     WHEN 'info'     THEN 4
		     ELSE 5
		 END, f.id ASC`,
		scanID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*SecurityFinding
	for rows.Next() {
		f, err := scanFindingRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func countSeverities(findings []*SecurityFinding) SeverityCounts {
	var c SeverityCounts
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			c.Critical++
		case SeverityHigh:
			c.High++
		case SeverityMedium:
			c.Medium++
		case SeverityLow:
			c.Low++
		case SeverityInfo:
			c.Info++
		}
	}
	return c
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(",?", n)[1:]
}
