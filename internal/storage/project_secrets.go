package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/cryptobox"
)

// ErrSecretRevoked is returned by Reveal when the row exists but
// has been marked revoked. Distinct from ErrNotFound so callers
// can render a "rotate this plugin's config" message rather than
// "secret never existed". Defined as a sentinel so resolver code
// can errors.Is() it without string-matching.
var ErrSecretRevoked = errors.New("project_secrets: secret revoked")

// ProjectSecret is one row in project_secrets — a named, encrypted
// value scoped to a project. The plaintext exists only in memory at
// create time and during resolution; the row carries AES-256-GCM
// (nonce, ciphertext) under the operator-supplied PLATYPUS_CA_KEK,
// the same KEK that wraps project CAs.
//
// Mutability: values are immutable. An operator who needs to change
// the value of, say, "datadog_api_key", revokes the old row and
// creates a new one — usually under the same name, since the partial
// UNIQUE index on (project_id, name) ignores revoked rows. Plugin
// configs that referenced the old secret_id keep working until they're
// re-saved against the new one; the resolver surfaces a clear error
// when a referenced secret is revoked.
type ProjectSecret struct {
	SecretID      string
	ProjectID     string
	Name          string
	Description   string
	Nonce         []byte // 12-byte AES-GCM nonce; persisted alongside ciphertext
	Ciphertext    []byte
	CreatedByUser string
	CreatedAt     time.Time
	LastUsedAt    *time.Time
	Revoked       bool
	RevokedAt     *time.Time
	RevokedByUser string
	RevokedReason string
}

// ProjectSecretRedacted is the safe-for-API view: identifies the
// secret and reports metadata without ever surfacing nonce or
// ciphertext bytes. Every list / get response uses this; the only
// path that materialises plaintext is the resolver, which logs a
// secret.use audit event each time.
type ProjectSecretRedacted struct {
	SecretID      string
	ProjectID     string
	Name          string
	Description   string
	CreatedByUser string
	CreatedAt     time.Time
	LastUsedAt    *time.Time
	Revoked       bool
	RevokedAt     *time.Time
}

func (s *ProjectSecret) Redacted() ProjectSecretRedacted {
	return ProjectSecretRedacted{
		SecretID:      s.SecretID,
		ProjectID:     s.ProjectID,
		Name:          s.Name,
		Description:   s.Description,
		CreatedByUser: s.CreatedByUser,
		CreatedAt:     s.CreatedAt,
		LastUsedAt:    s.LastUsedAt,
		Revoked:       s.Revoked,
		RevokedAt:     s.RevokedAt,
	}
}

// NewSecretID returns a fresh "sec_<hex>" identifier. 64 random bits
// is enough — secrets aren't credentials in their own right (knowing
// the id buys nothing without server-side decrypt access).
func NewSecretID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "sec_" + hex.EncodeToString(b[:]), nil
}

func (db *DB) ProjectSecrets() *ProjectSecretRepo {
	return &ProjectSecretRepo{db: db.DB}
}

type ProjectSecretRepo struct {
	db *sql.DB
}

// Create encrypts plaintext under the project KEK and inserts a new
// row. plaintext is wiped on the way out — the caller's caller should
// also clear its copy. Returns the persisted row (without re-decrypting
// the ciphertext) so the caller can echo the redacted shape back to
// the client.
func (r *ProjectSecretRepo) Create(ctx context.Context, projectID, name, description, byUser string, plaintext []byte) (*ProjectSecret, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("project_secrets: name required")
	}
	if len(plaintext) == 0 {
		return nil, errors.New("project_secrets: plaintext is empty")
	}
	nonce, ct, err := cryptobox.Seal(plaintext)
	if err != nil {
		return nil, err
	}
	id, err := NewSecretID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO project_secrets (
			secret_id, project_id, name, description,
			nonce, ciphertext,
			created_by_user, created_at,
			last_used_at, revoked, revoked_at, revoked_by_user, revoked_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, 0, NULL, NULL, NULL)`,
		id, projectID, strings.TrimSpace(name), nullableString(description),
		nonce, ct,
		nullableString(byUser), now,
	)
	if err != nil {
		return nil, err
	}
	return &ProjectSecret{
		SecretID:      id,
		ProjectID:     projectID,
		Name:          strings.TrimSpace(name),
		Description:   description,
		Nonce:         nonce,
		Ciphertext:    ct,
		CreatedByUser: byUser,
		CreatedAt:     now,
	}, nil
}

// Get fetches the encrypted row by id, including ciphertext + nonce
// for callers that need to decrypt. Use sparingly — most call sites
// should go through Reveal which folds in the decryption + audit
// stamp.
func (r *ProjectSecretRepo) Get(ctx context.Context, secretID string) (*ProjectSecret, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT secret_id, project_id, name, description,
		       nonce, ciphertext,
		       created_by_user, created_at,
		       last_used_at, revoked, revoked_at, revoked_by_user, revoked_reason
		  FROM project_secrets WHERE secret_id = ?`, secretID)
	s, err := scanProjectSecret(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

// Reveal looks up a secret, decrypts it, stamps last_used_at, and
// returns the plaintext. Refuses revoked rows — callers should
// surface "secret has been revoked, update plugin config" rather than
// silently using a stale value. The plaintext slice belongs to the
// caller; clear it as soon as the resolution step is done.
func (r *ProjectSecretRepo) Reveal(ctx context.Context, secretID string) ([]byte, error) {
	s, err := r.Get(ctx, secretID)
	if err != nil {
		return nil, err
	}
	if s.Revoked {
		return nil, ErrSecretRevoked
	}
	plaintext, err := cryptobox.Open(s.Nonce, s.Ciphertext)
	if err != nil {
		return nil, err
	}
	// Best-effort last_used_at — failure here doesn't fail the
	// resolution. The audit log is the authoritative use trail.
	_, _ = r.db.ExecContext(ctx, `
		UPDATE project_secrets SET last_used_at = ? WHERE secret_id = ?`,
		time.Now().UTC(), secretID,
	)
	return plaintext, nil
}

// ListByProject returns redacted views, newest first. Includes
// revoked rows so the UI can show "rotated 3 days ago" history; the
// caller filters by `Revoked == false` if it only wants live secrets.
func (r *ProjectSecretRepo) ListByProject(ctx context.Context, projectID string) ([]ProjectSecretRedacted, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT secret_id, project_id, name, description,
		       nonce, ciphertext,
		       created_by_user, created_at,
		       last_used_at, revoked, revoked_at, revoked_by_user, revoked_reason
		  FROM project_secrets
		 WHERE project_id = ?
		 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ProjectSecretRedacted
	for rows.Next() {
		s, err := scanProjectSecret(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s.Redacted())
	}
	return out, rows.Err()
}

// Revoke marks a secret revoked. Idempotent — revoking twice is fine.
// Returns ErrNotFound only when the row doesn't exist; revoking an
// already-revoked row is a no-op-success so callers can surface
// "revoked" uniformly.
func (r *ProjectSecretRepo) Revoke(ctx context.Context, secretID, byUser, reason string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE project_secrets
		   SET revoked = 1, revoked_at = ?, revoked_by_user = ?, revoked_reason = ?
		 WHERE secret_id = ? AND revoked = 0`,
		time.Now().UTC(), nullableString(byUser), nullableString(reason), secretID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Either the row doesn't exist or it was already revoked.
		// Get-then-decide so the handler can return 404 vs 200.
		if _, err := r.Get(ctx, secretID); errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
	}
	return nil
}

func scanProjectSecret(row rowScanner) (*ProjectSecret, error) {
	var (
		s          ProjectSecret
		desc       sql.NullString
		byUser     sql.NullString
		lastUsed   sql.NullTime
		revoked    int
		revokedAt  sql.NullTime
		revokedBy  sql.NullString
		revokedRea sql.NullString
	)
	err := row.Scan(
		&s.SecretID, &s.ProjectID, &s.Name, &desc,
		&s.Nonce, &s.Ciphertext,
		&byUser, &s.CreatedAt,
		&lastUsed, &revoked, &revokedAt, &revokedBy, &revokedRea,
	)
	if err != nil {
		return nil, err
	}
	s.Description = desc.String
	s.CreatedByUser = byUser.String
	if lastUsed.Valid {
		t := lastUsed.Time
		s.LastUsedAt = &t
	}
	s.Revoked = revoked == 1
	if revokedAt.Valid {
		t := revokedAt.Time
		s.RevokedAt = &t
	}
	s.RevokedByUser = revokedBy.String
	s.RevokedReason = revokedRea.String
	return &s, nil
}
