package storage

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"
)

// InstallDownloadToken backs one row in install_download_tokens. It is
// minted by an admin and consumed exactly once by `curl /api/v1/install/<id>`,
// at which point a fresh PAT gets minted atomically and embedded in
// the rendered bootstrap script. The plaintext secret only exists in
// memory at mint and at consume time; the stored column is SHA-256(secret).
type InstallDownloadToken struct {
	DownloadID          string
	SecretHash          []byte
	ProjectID           string
	IssuedByUser        string
	IssuedAt            time.Time
	ExpiresAt           time.Time
	TargetOS            string
	TargetArch          string
	ServerEndpoint      string
	PATTTLSeconds       int
	PATMaxUses          int
	PATBindingMachineID string
	PATDescription      string
	ConsumedAt          *time.Time
	ConsumedIP          string
	ConsumedUA          string
	ConsumedPATID       string
	// AutoApprove propagates into the PAT minted at consume time —
	// the resulting host enrolls straight to `approved` instead of
	// the default `pending`. See migration 000022.
	AutoApprove bool
	// PluginSpecs captures the operator's chosen baseline at mint
	// time: plugin_id + version + granted_capabilities +
	// config_overrides + schema_version per entry. Persisted as a
	// JSON array in the plugin_specs column. ConsumeInstallDownload
	// hands these to handler_enroll_v2 which stamps them onto the
	// host record so the agent-link reconciler can install each
	// plugin with full deployment intent.
	PluginSpecs []PluginSpec
	Revoked     bool
	RevokedAt         *time.Time
	RevokedByUser     string
	RevokedReason     string
}

// InstallDownloadStatus is derived (never materialised). Separate from
// EnrollmentStatus because "consumed" here means "the admin's `curl`
// happened", which is a different state than "the enrollment token was
// redeemed by an agent".
type InstallDownloadStatus string

const (
	InstallDownloadPending  InstallDownloadStatus = "pending"
	InstallDownloadConsumed InstallDownloadStatus = "consumed"
	InstallDownloadExpired  InstallDownloadStatus = "expired"
	InstallDownloadRevoked  InstallDownloadStatus = "revoked"
)

// Status resolves the token's state at the given wall clock.
func (t *InstallDownloadToken) Status(now time.Time) InstallDownloadStatus {
	switch {
	case t.Revoked:
		return InstallDownloadRevoked
	case t.ConsumedAt != nil:
		return InstallDownloadConsumed
	case !t.ExpiresAt.After(now):
		return InstallDownloadExpired
	default:
		return InstallDownloadPending
	}
}

func (db *DB) InstallDownloadTokens() *InstallDownloadTokenRepo {
	return &InstallDownloadTokenRepo{db: db.DB}
}

type InstallDownloadTokenRepo struct {
	db *sql.DB
}

// Create inserts a freshly-minted install download token. Caller hashes
// the plaintext secret before passing it in.
func (r *InstallDownloadTokenRepo) Create(ctx context.Context, t *InstallDownloadToken) error {
	if t.ServerEndpoint == "" {
		return errors.New("install_download_tokens: server_endpoint required")
	}
	if t.PATMaxUses <= 0 {
		t.PATMaxUses = 1
	}
	autoApprove := 0
	if t.AutoApprove {
		autoApprove = 1
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO install_download_tokens (
			download_id, secret_hash, project_id, issued_by_user,
			issued_at, expires_at, target_os, target_arch,
			server_endpoint, pat_ttl_seconds, pat_max_uses, pat_binding_machine_id,
			pat_description, auto_approve, plugin_specs,
			consumed_at, consumed_ip, consumed_ua,
			consumed_pat_id, revoked, revoked_at, revoked_by_user, revoked_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		          NULL, NULL, NULL, NULL, 0, NULL, NULL, NULL)`,
		t.DownloadID, t.SecretHash, t.ProjectID, t.IssuedByUser,
		t.IssuedAt.UTC(), t.ExpiresAt.UTC(),
		nullableString(t.TargetOS), nullableString(t.TargetArch),
		t.ServerEndpoint, t.PATTTLSeconds, t.PATMaxUses,
		nullableString(t.PATBindingMachineID),
		nullableString(t.PATDescription),
		autoApprove,
		encodeInstallTokenSpecs(t),
	)
	return err
}

// Get returns the row for a given download_id, or ErrNotFound.
func (r *InstallDownloadTokenRepo) Get(ctx context.Context, id string) (*InstallDownloadToken, error) {
	row := r.db.QueryRowContext(ctx, installDownloadColumns+` WHERE download_id = ?`, id)
	return scanInstallDownloadSingle(row)
}

// GetByConsumedPATID looks up the install token that was consumed to
// mint a given PAT id. Used at enroll time to recover the operator's
// baseline plugin allowlist (which is stored on the install token row,
// not the PAT) and copy it onto the freshly-created hosts row.
//
// Returns ErrNotFound when no install token claims that PAT — either
// the PAT was minted directly via /api/v1/pat-tokens (no install
// flow) or the join is broken.
func (r *InstallDownloadTokenRepo) GetByConsumedPATID(ctx context.Context, patID string) (*InstallDownloadToken, error) {
	row := r.db.QueryRowContext(ctx, installDownloadColumns+` WHERE consumed_pat_id = ?`, patID)
	return scanInstallDownloadSingle(row)
}

// ListByProject returns install tokens for a project, newest first.
// `includeInactive=false` hides revoked rows but NOT expired or consumed ones
// (the distinction between "never used" and "used" matters for admins more
// than the revoked flag).
func (r *InstallDownloadTokenRepo) ListByProject(ctx context.Context, projectID string, includeInactive bool) ([]*InstallDownloadToken, error) {
	q := installDownloadColumns + ` WHERE project_id = ?`
	args := []any{projectID}
	if !includeInactive {
		q += ` AND revoked = 0`
	}
	q += ` ORDER BY issued_at DESC`

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*InstallDownloadToken
	for rows.Next() {
		t, err := scanInstallDownloadToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TryConsume atomically marks a download token as consumed if it is
// still redeemable. Returns a status string for audit; callers must log
// the returned outcome verbatim whether or not consumed == nil. On
// "success" the token's consumed_at / consumed_ip / consumed_ua / consumed_pat_id
// columns are persisted and the returned struct reflects that.
//
// The patID parameter is the id of the PAT that the enrollment service
// just minted — it's passed in here so the two rows are linked inside
// the same transaction, ensuring the mint and the consume either both
// happen or neither does.
func (r *InstallDownloadTokenRepo) TryConsume(
	ctx context.Context,
	downloadID string,
	secret []byte,
	clientIP, clientUA, patID string,
	now time.Time,
) (*InstallDownloadToken, string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	t, err := scanInstallDownloadSingle(tx.QueryRowContext(ctx,
		installDownloadColumns+` WHERE download_id = ?`, downloadID))
	if errors.Is(err, ErrNotFound) {
		return nil, "unknown_id", nil
	}
	if err != nil {
		return nil, "", err
	}

	if subtle.ConstantTimeCompare(sha256Sum(secret), t.SecretHash) != 1 {
		return t, "invalid_secret", nil
	}
	if t.Revoked {
		return t, "revoked", nil
	}
	if t.ConsumedAt != nil {
		return t, "already_consumed", nil
	}
	if !t.ExpiresAt.After(now) {
		return t, "expired", nil
	}

	// Atomic commit. The WHERE clause re-asserts liveness so two racing
	// transactions can't both win — only one ends up with RowsAffected=1.
	res, err := tx.ExecContext(ctx, `
		UPDATE install_download_tokens
		   SET consumed_at = ?, consumed_ip = ?, consumed_ua = ?, consumed_pat_id = ?
		 WHERE download_id = ?
		   AND revoked = 0
		   AND consumed_at IS NULL
		   AND expires_at > ?`,
		now.UTC(),
		nullableString(clientIP),
		nullableString(clientUA),
		nullableString(patID),
		downloadID, now.UTC())
	if err != nil {
		return nil, "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, "", err
	}
	if n == 0 {
		return t, "already_consumed", nil
	}

	nowPtr := now.UTC()
	t.ConsumedAt = &nowPtr
	t.ConsumedIP = clientIP
	t.ConsumedUA = clientUA
	t.ConsumedPATID = patID

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	rollback = false
	return t, "success", nil
}

// Revoke marks the row revoked. Idempotent; revoking something that's
// already consumed is accepted but records revoked_at for audit.
func (r *InstallDownloadTokenRepo) Revoke(ctx context.Context, id, byUser, reason string, at time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE install_download_tokens
		   SET revoked = 1, revoked_at = ?, revoked_by_user = ?, revoked_reason = ?
		 WHERE download_id = ? AND revoked = 0`,
		at.UTC(), byUser, nullableString(reason), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		if _, err := r.Get(ctx, id); errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
	}
	return nil
}

const installDownloadColumns = `
	SELECT download_id, secret_hash, project_id, issued_by_user,
	       issued_at, expires_at, target_os, target_arch,
	       server_endpoint, pat_ttl_seconds, pat_max_uses, pat_binding_machine_id,
	       pat_description, auto_approve, plugin_specs,
	       consumed_at, consumed_ip, consumed_ua,
	       consumed_pat_id, revoked, revoked_at, revoked_by_user, revoked_reason
	  FROM install_download_tokens`

func scanInstallDownloadSingle(row rowScanner) (*InstallDownloadToken, error) {
	t, err := scanInstallDownloadToken(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func scanInstallDownloadToken(row rowScanner) (*InstallDownloadToken, error) {
	var (
		t            InstallDownloadToken
		tOS          sql.NullString
		tArch        sql.NullString
		bindMID      sql.NullString
		patDesc      sql.NullString
		autoApprove  int
		baselineCSV  sql.NullString
		consAt       sql.NullTime
		consIP       sql.NullString
		consUA       sql.NullString
		consPAT      sql.NullString
		revAt        sql.NullTime
		revBy        sql.NullString
		revReas      sql.NullString
		revoked      int
	)
	err := row.Scan(
		&t.DownloadID, &t.SecretHash, &t.ProjectID, &t.IssuedByUser,
		&t.IssuedAt, &t.ExpiresAt, &tOS, &tArch,
		&t.ServerEndpoint, &t.PATTTLSeconds, &t.PATMaxUses, &bindMID,
		&patDesc, &autoApprove, &baselineCSV,
		&consAt, &consIP, &consUA,
		&consPAT, &revoked, &revAt, &revBy, &revReas,
	)
	if err != nil {
		return nil, err
	}
	t.TargetOS = tOS.String
	t.TargetArch = tArch.String
	t.PATBindingMachineID = bindMID.String
	t.PATDescription = patDesc.String
	t.AutoApprove = autoApprove == 1
	// Both views of the same JSON column are populated on read so
	if specs, err := DecodePluginSpecs(baselineCSV.String); err == nil {
		t.PluginSpecs = specs
	}
	if consAt.Valid {
		v := consAt.Time
		t.ConsumedAt = &v
	}
	t.ConsumedIP = consIP.String
	t.ConsumedUA = consUA.String
	t.ConsumedPATID = consPAT.String
	t.Revoked = revoked == 1
	if revAt.Valid {
		v := revAt.Time
		t.RevokedAt = &v
	}
	t.RevokedByUser = revBy.String
	t.RevokedReason = revReas.String
	return &t, nil
}

// encodeInstallTokenSpecs picks the right value to persist into the
// plugin_specs JSON column. Rich PluginSpecs wins when supplied —
// encodeInstallTokenSpecs persists the install token's PluginSpecs
// into the plugin_specs JSON column. Empty slice → "" so the
// column reads back as "no baseline".
func encodeInstallTokenSpecs(t *InstallDownloadToken) string {
	if len(t.PluginSpecs) == 0 {
		return ""
	}
	raw, err := EncodePluginSpecs(t.PluginSpecs)
	if err != nil || raw == nil {
		return ""
	}
	s, _ := raw.(string)
	return s
}

// sha256Sum is a small helper so callers (and tests) don't need to
// import crypto/sha256 just to do one hash.
func sha256Sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

// --- install_download_events -------------------------------------------

// InstallDownloadEvent logs one curl / consume attempt. No FK back to
// install_download_tokens: failed attempts against nonexistent ids
// (scanning / brute force) must still land here so we can detect them.
type InstallDownloadEvent struct {
	ID          int64
	At          time.Time
	DownloadID  string
	ClientIP    string
	ClientUA    string
	PATTokenID  string
	Outcome     string
	ErrorDetail string
}

func (db *DB) InstallDownloadEvents() *InstallDownloadEventRepo {
	return &InstallDownloadEventRepo{db: db.DB}
}

type InstallDownloadEventRepo struct {
	db *sql.DB
}

// Record appends one event row. Matches the PAT redemption pattern —
// best-effort, silent on failure for callers who don't care.
func (r *InstallDownloadEventRepo) Record(ctx context.Context, e *InstallDownloadEvent) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO install_download_events
			(at, download_id, client_ip, client_ua, pat_token_id, outcome, error_detail)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.At.UTC(), e.DownloadID,
		nullableString(e.ClientIP),
		nullableString(e.ClientUA),
		nullableString(e.PATTokenID),
		e.Outcome,
		nullableString(e.ErrorDetail),
	)
	return err
}

// ListByDownload returns events for a given download_id, newest first.
func (r *InstallDownloadEventRepo) ListByDownload(ctx context.Context, id string, limit int) ([]*InstallDownloadEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, at, download_id, client_ip, client_ua, pat_token_id, outcome, error_detail
		  FROM install_download_events
		 WHERE download_id = ?
		 ORDER BY at DESC
		 LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []*InstallDownloadEvent
	for rows.Next() {
		e := &InstallDownloadEvent{}
		var clientIP, clientUA, patID, errDetail sql.NullString
		if err := rows.Scan(&e.ID, &e.At, &e.DownloadID, &clientIP, &clientUA, &patID, &e.Outcome, &errDetail); err != nil {
			return nil, err
		}
		e.ClientIP = clientIP.String
		e.ClientUA = clientUA.String
		e.PATTokenID = patID.String
		e.ErrorDetail = errDetail.String
		out = append(out, e)
	}
	return out, rows.Err()
}

// Use a no-op closure here to satisfy the linter: `subtle` is referenced
// only inside TryConsume above. Keep the import aligned with reality.
var _ = subtle.ConstantTimeCompare
