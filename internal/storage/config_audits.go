package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Risk values for ConfigLeak.Risk. Mirrors the agent-side
// config_audit.Risk constants. Stored as lowercase tokens.
const (
	RiskInfo   = "info"
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

// AUDIT_KEEP_PER_HOST caps the per-host audit history. Older audits
// are pruned in the same transaction that inserts a new one. Mirrors
// SCAN_KEEP_PER_HOST so operators see consistent retention behaviour
// across the two surfaces.
const AUDIT_KEEP_PER_HOST = 30

// ConfigAudit is one row in host_config_audits. The denormalised
// leak-count fields mirror the actual rows in host_config_leaks; both
// Save and the UI rely on them so the fleet/host-list endpoint reads
// them without joining N rows.
type ConfigAudit struct {
	ID            string
	ProjectID     string
	HostID        string
	StartedAtUnix int64
	ElapsedMs     int64
	Error         string
	RiskCounts    RiskCounts
	AuditorsJSON  string
}

// ConfigLeak is one row in host_config_leaks. MatchRedacted is
// already-masked content as it arrived from the agent — the storage
// layer never sees plaintext credentials.
type ConfigLeak struct {
	ID             string
	AuditID        string
	HostID         string
	ProjectID      string
	LeakID         string
	AuditorID      string
	Category       string
	Risk           string
	Title          string
	Location       string
	MatchRedacted  string
	Pattern        string
	Description    string
	Remediation    string
	ReferencesJSON string
	ScannedAtUnix  int64
}

// RiskCounts is a per-risk histogram, denormalised on the audit row
// and returned to the UI as a fleet summary.
type RiskCounts struct {
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
	Info   int `json:"info"`
}

// AuditSummary is the per-host snapshot the hosts list endpoint
// returns for the Config tab badges. Hosts that have never been
// audited are absent from the map ("never audited" must stay UI-
// distinct from "audited and clean").
type AuditSummary struct {
	AuditID       string     `json:"audit_id"`
	StartedAtUnix int64      `json:"started_at_unix"`
	Counts        RiskCounts `json:"counts"`
}

// ListLeaksFilter is the search shape for the project-level Config
// page. Empty slices = no filter on that axis. HostID = "" = all
// hosts. Q is case-insensitive substring match on title and location.
type ListLeaksFilter struct {
	Risk     []string
	Category []string
	HostID   string
	Q        string
}

// ConfigAuditsRepo owns reads and writes against host_config_audits +
// host_config_leaks.
type ConfigAuditsRepo struct {
	db *sql.DB
}

// ConfigAudits returns the repo accessor in the same shape as the
// existing SecurityScans() accessor on *DB.
func (db *DB) ConfigAudits() *ConfigAuditsRepo {
	return &ConfigAuditsRepo{db: db.DB}
}

// Save inserts one audit and its leaks in a single transaction. The
// denormalised risk counts on ConfigAudit are recomputed from the
// supplied leaks rather than trusted from the caller — that way the
// audit row and the leaks table can never disagree.
//
// As part of the same transaction, audits for HostID older than
// AUDIT_KEEP_PER_HOST are deleted.
func (r *ConfigAuditsRepo) Save(ctx context.Context, audit *ConfigAudit, leaks []*ConfigLeak) error {
	if audit == nil {
		return fmt.Errorf("config_audits: Save with nil audit")
	}
	audit.RiskCounts = countRisks(leaks)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("config_audits: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO host_config_audits (
			id, project_id, host_id, started_at_unix, elapsed_ms, error,
			leak_count_high, leak_count_medium, leak_count_low, leak_count_info,
			auditors_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		audit.ID, audit.ProjectID, audit.HostID, audit.StartedAtUnix, audit.ElapsedMs, audit.Error,
		audit.RiskCounts.High, audit.RiskCounts.Medium, audit.RiskCounts.Low, audit.RiskCounts.Info,
		audit.AuditorsJSON,
	); err != nil {
		return fmt.Errorf("config_audits: insert audit: %w", err)
	}

	for _, l := range leaks {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO host_config_leaks (
				id, audit_id, host_id, project_id, leak_id, auditor_id, category,
				risk, title, location, match_redacted, pattern, description, remediation,
				references_json, scanned_at_unix
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			l.ID, audit.ID, audit.HostID, audit.ProjectID, l.LeakID, l.AuditorID, l.Category,
			l.Risk, l.Title, l.Location, l.MatchRedacted, l.Pattern, l.Description, l.Remediation,
			l.ReferencesJSON, audit.StartedAtUnix,
		); err != nil {
			return fmt.Errorf("config_audits: insert leak %s: %w", l.LeakID, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM host_config_audits
		 WHERE host_id = ?
		   AND id NOT IN (
		       SELECT id FROM host_config_audits
		        WHERE host_id = ?
		        ORDER BY started_at_unix DESC
		        LIMIT ?
		   )`,
		audit.HostID, audit.HostID, AUDIT_KEEP_PER_HOST,
	); err != nil {
		return fmt.Errorf("config_audits: prune retention: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("config_audits: commit: %w", err)
	}
	return nil
}

// LatestForHost returns the most recent audit + its leaks for hostID.
// Returns ErrNotFound when the host has never been audited (UI maps
// to a 404 to distinguish never-audited from audited-clean).
func (r *ConfigAuditsRepo) LatestForHost(ctx context.Context, hostID string) (*ConfigAudit, []*ConfigLeak, error) {
	audit, err := r.latestAuditRow(ctx, hostID)
	if err != nil {
		return nil, nil, err
	}
	leaks, err := r.leaksForAudit(ctx, audit.ID)
	if err != nil {
		return nil, nil, err
	}
	return audit, leaks, nil
}

// GetAudit returns a specific audit + leaks by id. Returns ErrNotFound
// when the audit id is unknown.
func (r *ConfigAuditsRepo) GetAudit(ctx context.Context, auditID string) (*ConfigAudit, []*ConfigLeak, error) {
	row := r.db.QueryRowContext(ctx, auditSelectColumns+` FROM host_config_audits WHERE id = ?`, auditID)
	a, err := scanAuditRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	leaks, err := r.leaksForAudit(ctx, auditID)
	if err != nil {
		return nil, nil, err
	}
	return a, leaks, nil
}

// ListAuditsForHost returns lightweight audit rows newest-first, no
// leaks attached. Used by the per-host History dropdown.
func (r *ConfigAuditsRepo) ListAuditsForHost(ctx context.Context, hostID string, limit int) ([]*ConfigAudit, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		auditSelectColumns+` FROM host_config_audits WHERE host_id = ? ORDER BY started_at_unix DESC LIMIT ?`,
		hostID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*ConfigAudit
	for rows.Next() {
		a, err := scanAuditRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// LatestSummariesForProject returns one entry per host in projectID,
// keyed by host_id, populated from each host's most recent audit.
// Hosts with no audits are absent from the map.
func (r *ConfigAuditsRepo) LatestSummariesForProject(ctx context.Context, projectID string) (map[string]AuditSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.host_id, a.id, a.started_at_unix,
		       a.leak_count_high, a.leak_count_medium,
		       a.leak_count_low, a.leak_count_info
		  FROM host_config_audits a
		 WHERE a.project_id = ?
		   AND a.started_at_unix = (
		       SELECT MAX(started_at_unix) FROM host_config_audits
		        WHERE host_id = a.host_id
		   )`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]AuditSummary{}
	for rows.Next() {
		var (
			hostID  string
			summary AuditSummary
		)
		if err := rows.Scan(
			&hostID, &summary.AuditID, &summary.StartedAtUnix,
			&summary.Counts.High, &summary.Counts.Medium,
			&summary.Counts.Low, &summary.Counts.Info,
		); err != nil {
			return nil, err
		}
		out[hostID] = summary
	}
	return out, rows.Err()
}

// ListLeaks powers the project-level Config page. Restricted to the
// latest audit per host so the fleet view always reflects current
// posture.
func (r *ConfigAuditsRepo) ListLeaks(ctx context.Context, projectID string, filter ListLeaksFilter, page Page) ([]*ConfigLeak, int, error) {
	conds := []string{"l.project_id = ?"}
	args := []any{projectID}

	conds = append(conds, `l.audit_id = (
		SELECT id FROM host_config_audits a
		 WHERE a.host_id = l.host_id
		 ORDER BY a.started_at_unix DESC
		 LIMIT 1
	)`)

	if len(filter.Risk) > 0 {
		conds = append(conds, "l.risk IN ("+placeholders(len(filter.Risk))+")")
		for _, s := range filter.Risk {
			args = append(args, s)
		}
	}
	if len(filter.Category) > 0 {
		conds = append(conds, "l.category IN ("+placeholders(len(filter.Category))+")")
		for _, c := range filter.Category {
			args = append(args, c)
		}
	}
	if filter.HostID != "" {
		conds = append(conds, "l.host_id = ?")
		args = append(args, filter.HostID)
	}
	if q := strings.TrimSpace(filter.Q); q != "" {
		conds = append(conds, "(LOWER(l.title) LIKE ? OR LOWER(l.location) LIKE ?)")
		needle := "%" + strings.ToLower(q) + "%"
		args = append(args, needle, needle)
	}

	where := strings.Join(conds, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM host_config_leaks l WHERE `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	_, size, offset := page.effective()
	args = append(args, size, offset)
	rows, err := r.db.QueryContext(ctx,
		leakSelectColumns+` FROM host_config_leaks l
		 WHERE `+where+`
		 ORDER BY CASE l.risk
		     WHEN 'high'   THEN 0
		     WHEN 'medium' THEN 1
		     WHEN 'low'    THEN 2
		     WHEN 'info'   THEN 3
		     ELSE 4
		 END,
		 l.scanned_at_unix DESC,
		 l.id ASC
		 LIMIT ? OFFSET ?`, args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	var out []*ConfigLeak
	for rows.Next() {
		l, err := scanLeakRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// --- internals ----------------------------------------------------

const auditSelectColumns = `SELECT
	id, project_id, host_id, started_at_unix, elapsed_ms, error,
	leak_count_high, leak_count_medium, leak_count_low, leak_count_info,
	auditors_json`

const leakSelectColumns = `SELECT
	l.id, l.audit_id, l.host_id, l.project_id, l.leak_id, l.auditor_id,
	l.category, l.risk, l.title, l.location, l.match_redacted, l.pattern,
	l.description, l.remediation, l.references_json, l.scanned_at_unix`

func scanAuditRow(row interface{ Scan(...any) error }) (*ConfigAudit, error) {
	var a ConfigAudit
	if err := row.Scan(
		&a.ID, &a.ProjectID, &a.HostID, &a.StartedAtUnix, &a.ElapsedMs, &a.Error,
		&a.RiskCounts.High, &a.RiskCounts.Medium, &a.RiskCounts.Low, &a.RiskCounts.Info,
		&a.AuditorsJSON,
	); err != nil {
		return nil, err
	}
	return &a, nil
}

func scanLeakRow(row interface{ Scan(...any) error }) (*ConfigLeak, error) {
	var l ConfigLeak
	if err := row.Scan(
		&l.ID, &l.AuditID, &l.HostID, &l.ProjectID, &l.LeakID, &l.AuditorID,
		&l.Category, &l.Risk, &l.Title, &l.Location, &l.MatchRedacted, &l.Pattern,
		&l.Description, &l.Remediation, &l.ReferencesJSON, &l.ScannedAtUnix,
	); err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *ConfigAuditsRepo) latestAuditRow(ctx context.Context, hostID string) (*ConfigAudit, error) {
	row := r.db.QueryRowContext(ctx,
		auditSelectColumns+` FROM host_config_audits WHERE host_id = ? ORDER BY started_at_unix DESC LIMIT 1`,
		hostID,
	)
	a, err := scanAuditRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return a, nil
}

func (r *ConfigAuditsRepo) leaksForAudit(ctx context.Context, auditID string) ([]*ConfigLeak, error) {
	rows, err := r.db.QueryContext(ctx,
		leakSelectColumns+` FROM host_config_leaks l WHERE l.audit_id = ?
		 ORDER BY CASE l.risk
		     WHEN 'high'   THEN 0
		     WHEN 'medium' THEN 1
		     WHEN 'low'    THEN 2
		     WHEN 'info'   THEN 3
		     ELSE 4
		 END, l.id ASC`,
		auditID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*ConfigLeak
	for rows.Next() {
		l, err := scanLeakRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func countRisks(leaks []*ConfigLeak) RiskCounts {
	var c RiskCounts
	for _, l := range leaks {
		switch l.Risk {
		case RiskHigh:
			c.High++
		case RiskMedium:
			c.Medium++
		case RiskLow:
			c.Low++
		case RiskInfo:
			c.Info++
		}
	}
	return c
}
