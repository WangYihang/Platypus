package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

// EnrollmentPreset mirrors one row in enrollment_presets. It captures
// the choices an operator makes in the Enroll Agent wizard so the next
// enrolment for the same shape collapses to "pick preset → Generate".
//
// Nothing here is authentication material — every preset use still
// mints a fresh one-shot install token via the install-artifact flow.
// This row is purely a saved input template.
type EnrollmentPreset struct {
	PresetID            string
	ProjectID           string
	Name                string
	Description         string
	ServerEndpoint      string
	TargetOS            string
	TargetArch          string
	TTLSeconds          *int
	PATMaxUses          *int
	AutoApprove         bool
	SkipTLSVerification bool
	BaselinePluginIDs   []string
	PATDescription      string
	// IsSeed flags the three system defaults that are inserted on
	// first wizard open of a fresh project. Operators can still
	// edit / delete them; the flag exists so the UI can render a
	// "system default" badge and so the seed step skips re-creation
	// once the project already has any presets.
	IsSeed        bool
	CreatedByUser string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (db *DB) EnrollmentPresets() *EnrollmentPresetRepo {
	return &EnrollmentPresetRepo{db: db.DB}
}

type EnrollmentPresetRepo struct {
	db *sql.DB
}

// SystemPresetSpec is the immutable, code-side definition of one of
// the three system-default presets. Seeding skips entries whose
// (os, arch) the live install manifest doesn't publish.
type SystemPresetSpec struct {
	Name     string
	OS       string
	Arch     string
	Comment  string
}

// SystemPresetSpecs is the canonical list of presets seeded into a
// fresh project on first wizard open. Conservative defaults across
// the board so the operator picks an explicit "less safe" override
// rather than getting one for free.
var SystemPresetSpecs = []SystemPresetSpec{
	{Name: "Linux x86_64", OS: "linux", Arch: "amd64", Comment: "Default seed: Linux servers / containers."},
	{Name: "Windows x64", OS: "windows", Arch: "amd64", Comment: "Default seed: Windows hosts."},
	{Name: "macOS Apple Silicon", OS: "darwin", Arch: "arm64", Comment: "Default seed: macOS arm64 (M-series)."},
}

// SeedPlatform is the (os, arch) pair the seed step filters against.
// Mirrors the install-platforms manifest shape without a cross-package
// dependency.
type SeedPlatform struct {
	OS   string
	Arch string
}

// NewPresetID returns a fresh "epr_<hex>" identifier. Presets aren't
// security-sensitive so a 64-bit random tail is fine; the prefix keeps
// id strings self-describing in logs and audit rows.
func NewPresetID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "epr_" + hex.EncodeToString(b[:]), nil
}

// Create inserts a single preset row.
func (r *EnrollmentPresetRepo) Create(ctx context.Context, p *EnrollmentPreset) error {
	pluginsJSON, err := encodePluginIDs(p.BaselinePluginIDs)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO enrollment_presets (
			preset_id, project_id, name, description,
			server_endpoint, target_os, target_arch,
			ttl_seconds, pat_max_uses,
			auto_approve, skip_tls_verification,
			baseline_plugin_ids, pat_description,
			is_seed, created_by_user, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.PresetID, p.ProjectID, p.Name, nullableString(p.Description),
		nullableString(p.ServerEndpoint), nullableString(p.TargetOS), nullableString(p.TargetArch),
		nullableIntPtr(p.TTLSeconds), nullableIntPtr(p.PATMaxUses),
		boolToInt(p.AutoApprove), boolToInt(p.SkipTLSVerification),
		pluginsJSON, nullableString(p.PATDescription),
		boolToInt(p.IsSeed), nullableString(p.CreatedByUser),
		p.CreatedAt.UTC(), p.UpdatedAt.UTC(),
	)
	return err
}

// Get fetches one preset by its preset_id. Returns ErrNotFound on miss.
func (r *EnrollmentPresetRepo) Get(ctx context.Context, presetID string) (*EnrollmentPreset, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT preset_id, project_id, name, description,
		       server_endpoint, target_os, target_arch,
		       ttl_seconds, pat_max_uses,
		       auto_approve, skip_tls_verification,
		       baseline_plugin_ids, pat_description,
		       is_seed, created_by_user, created_at, updated_at
		  FROM enrollment_presets WHERE preset_id = ?`, presetID)
	p, err := scanEnrollmentPreset(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// ListByProject returns presets in a project, newest first.
func (r *EnrollmentPresetRepo) ListByProject(ctx context.Context, projectID string) ([]*EnrollmentPreset, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT preset_id, project_id, name, description,
		       server_endpoint, target_os, target_arch,
		       ttl_seconds, pat_max_uses,
		       auto_approve, skip_tls_verification,
		       baseline_plugin_ids, pat_description,
		       is_seed, created_by_user, created_at, updated_at
		  FROM enrollment_presets
		 WHERE project_id = ?
		 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*EnrollmentPreset
	for rows.Next() {
		p, err := scanEnrollmentPreset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Update overwrites a preset's mutable fields. preset_id, project_id,
// is_seed, created_by_user, and created_at are immutable — operators
// rename and tweak settings; provenance and "is this a seed?" don't
// change after creation. Returns ErrNotFound if the row vanished.
func (r *EnrollmentPresetRepo) Update(ctx context.Context, p *EnrollmentPreset) error {
	pluginsJSON, err := encodePluginIDs(p.BaselinePluginIDs)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE enrollment_presets
		   SET name = ?, description = ?,
		       server_endpoint = ?, target_os = ?, target_arch = ?,
		       ttl_seconds = ?, pat_max_uses = ?,
		       auto_approve = ?, skip_tls_verification = ?,
		       baseline_plugin_ids = ?, pat_description = ?,
		       updated_at = ?
		 WHERE preset_id = ?`,
		p.Name, nullableString(p.Description),
		nullableString(p.ServerEndpoint), nullableString(p.TargetOS), nullableString(p.TargetArch),
		nullableIntPtr(p.TTLSeconds), nullableIntPtr(p.PATMaxUses),
		boolToInt(p.AutoApprove), boolToInt(p.SkipTLSVerification),
		pluginsJSON, nullableString(p.PATDescription),
		p.UpdatedAt.UTC(),
		p.PresetID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a preset row. Returns ErrNotFound if it was already
// gone.
func (r *EnrollmentPresetRepo) Delete(ctx context.Context, presetID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM enrollment_presets WHERE preset_id = ?`, presetID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SeedSystemPresets inserts the system-default presets that don't
// already exist in the project, filtered by what the live install
// manifest publishes. Returns the project's preset list after the
// inserts (ordered newest-first per ListByProject).
//
// Seeding is intentionally lazy and idempotent:
//
//   - Lazy: we don't seed at project creation time so projects that
//     never enroll any agents stay empty.
//   - Idempotent: ON the (project_id, name) UNIQUE index — re-runs
//     are a no-op for already-seeded rows. A row deleted by an
//     operator stays deleted as long as any other preset exists in
//     the project (the handler only calls Seed when the list is
//     empty), which preserves "I removed Windows on purpose" intent.
func (r *EnrollmentPresetRepo) SeedSystemPresets(
	ctx context.Context,
	projectID string,
	supported []SeedPlatform,
	now time.Time,
	byUser string,
) ([]*EnrollmentPreset, error) {
	supportedSet := make(map[string]bool, len(supported))
	for _, s := range supported {
		supportedSet[s.OS+"/"+s.Arch] = true
	}
	for _, spec := range SystemPresetSpecs {
		if len(supported) > 0 && !supportedSet[spec.OS+"/"+spec.Arch] {
			// Manifest is published but doesn't carry this (os, arch).
			// Skip — seeding a card the operator can never actually
			// install from is just clutter.
			continue
		}
		id, err := NewPresetID()
		if err != nil {
			return nil, err
		}
		ttl := 300 // 5 minutes — same default as the wizard's TTL step
		maxUses := 1
		// INSERT OR IGNORE leans on the (project_id, name) UNIQUE
		// index. A second concurrent caller racing the seed step ends
		// up no-op'd cleanly instead of duplicating rows.
		_, err = r.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO enrollment_presets (
				preset_id, project_id, name, description,
				server_endpoint, target_os, target_arch,
				ttl_seconds, pat_max_uses,
				auto_approve, skip_tls_verification,
				baseline_plugin_ids, pat_description,
				is_seed, created_by_user, created_at, updated_at
			) VALUES (?, ?, ?, ?, NULL, ?, ?, ?, ?, 0, 1, NULL, NULL, 1, ?, ?, ?)`,
			id, projectID, spec.Name, nullableString(spec.Comment),
			spec.OS, spec.Arch,
			ttl, maxUses,
			nullableString(byUser), now.UTC(), now.UTC(),
		)
		if err != nil {
			return nil, err
		}
	}
	return r.ListByProject(ctx, projectID)
}

// scanEnrollmentPreset reads one row out of either *sql.Row or
// *sql.Rows via the shared rowScanner interface used elsewhere in
// this package.
func scanEnrollmentPreset(row rowScanner) (*EnrollmentPreset, error) {
	var (
		p           EnrollmentPreset
		desc        sql.NullString
		serverEp    sql.NullString
		os          sql.NullString
		arch        sql.NullString
		ttl         sql.NullInt64
		maxUses     sql.NullInt64
		autoApprove int
		skipTLS     int
		plugins     sql.NullString
		patDesc     sql.NullString
		isSeed      int
		byUser      sql.NullString
	)
	err := row.Scan(
		&p.PresetID, &p.ProjectID, &p.Name, &desc,
		&serverEp, &os, &arch,
		&ttl, &maxUses,
		&autoApprove, &skipTLS,
		&plugins, &patDesc,
		&isSeed, &byUser, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Description = desc.String
	p.ServerEndpoint = serverEp.String
	p.TargetOS = os.String
	p.TargetArch = arch.String
	if ttl.Valid {
		v := int(ttl.Int64)
		p.TTLSeconds = &v
	}
	if maxUses.Valid {
		v := int(maxUses.Int64)
		p.PATMaxUses = &v
	}
	p.AutoApprove = autoApprove == 1
	p.SkipTLSVerification = skipTLS == 1
	if plugins.Valid && plugins.String != "" {
		ids, err := decodePluginIDs(plugins.String)
		if err != nil {
			return nil, err
		}
		p.BaselinePluginIDs = ids
	}
	p.PATDescription = patDesc.String
	p.IsSeed = isSeed == 1
	p.CreatedByUser = byUser.String
	return &p, nil
}

func encodePluginIDs(ids []string) (interface{}, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func decodePluginIDs(s string) ([]string, error) {
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// nullableIntPtr maps a *int to a SQL value: nil → NULL, non-nil →
// the int. Mirrors nullableString but for the optional integer
// columns ttl_seconds / pat_max_uses where "unset" must round-trip
// as NULL rather than 0.
func nullableIntPtr(p *int) interface{} {
	if p == nil {
		return nil
	}
	return *p
}
