// Package enrollment handles the PAT / session-token credential lifecycle:
// minting new PATs, redeeming them into agent sessions, and rotating
// existing sessions. It is the single source of truth for how the
// opaque `plt_*` / `sess_*` token strings are parsed, hashed, and
// verified.
//
// The package deliberately sits above internal/storage but below
// internal/api and internal/core so it can be called from either the
// admin REST layer (minting) or the agent-facing TCP handshake
// (redeeming) without creating a cycle.
package enrollment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/WangYihang/Platypus/internal/storage"
)

const (
	// PATPrefix tags tokens issued as one-shot provisioning credentials.
	// Visible in logs / git / Slack so secret scanners can match it the
	// same way they do GitHub's `ghp_`.
	PATPrefix = "plt_"
	// SessionPrefix tags long-lived rotating credentials issued to an
	// agent after successful enrollment.
	SessionPrefix = "sess_"

	// Token shape: "<prefix><id>.<secret>". The id half is the primary
	// key in pat_tokens / agent_sessions (indexed); the secret half is
	// what we hash and compare against the stored SHA-256 digest.
	idLen     = 20
	secretLen = 20

	// DefaultPATTTL is applied when an issue request leaves ttl_seconds
	// unset. Short enough that an accidentally-leaked token burns out
	// quickly; long enough to survive manual copy-paste flows.
	DefaultPATTTL = 1 * time.Hour

	// DefaultSessionTTL is the lifetime of a freshly-issued session
	// token. Rotations reset this.
	DefaultSessionTTL = 30 * 24 * time.Hour

	// RenewGrace is how long before expiry the server recommends the
	// agent rotate its session. Leaves headroom for transient network
	// partitions so rotation doesn't race against hard expiry.
	RenewGrace = 6 * time.Hour
)

// enc is unpadded lowercase base32 — URL-safe, case-insensitive, and
// (critically for test stability and log grep) free of confusable chars
// like I/l/0/O once decoded to lowercase.
var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// CredentialKind classifies which half of the lifecycle a caller is
// presenting. The server dispatches on this after parseCredential.
type CredentialKind int

const (
	KindUnknown CredentialKind = iota
	KindPAT
	KindSession
)

// ParsedCredential is what Parse returns: a kind-tagged (id, secret)
// pair. We don't expose a combined "token" string to the rest of the
// codebase — once parsed, the two halves flow separately so nothing
// accidentally logs the whole thing.
type ParsedCredential struct {
	Kind   CredentialKind
	ID     string // e.g. "plt_abc..."
	Secret []byte // raw bytes after base32 decode
}

// ErrMalformed is returned by Parse for any string that doesn't match
// the expected `<prefix><id>.<secret>` shape.
var ErrMalformed = errors.New("enrollment: malformed credential")

// Parse splits a presented credential. It does NOT validate the
// credential against storage — that's RedeemPAT / RedeemSession.
func Parse(raw string) (*ParsedCredential, error) {
	var kind CredentialKind
	var rest string
	switch {
	case strings.HasPrefix(raw, PATPrefix):
		kind = KindPAT
		rest = strings.TrimPrefix(raw, PATPrefix)
	case strings.HasPrefix(raw, SessionPrefix):
		kind = KindSession
		rest = strings.TrimPrefix(raw, SessionPrefix)
	default:
		return nil, ErrMalformed
	}
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrMalformed
	}
	secret, err := enc.DecodeString(strings.ToUpper(parts[1]))
	if err != nil {
		return nil, ErrMalformed
	}
	// We carry the full id (including prefix) back out so callers can
	// round-trip it to the storage layer verbatim.
	id := prefixFor(kind) + parts[0]
	return &ParsedCredential{Kind: kind, ID: id, Secret: secret}, nil
}

func prefixFor(k CredentialKind) string {
	if k == KindSession {
		return SessionPrefix
	}
	return PATPrefix
}

// generate builds a fresh (id, secret, full) triple for a given prefix.
// Callers store SHA-256(secret) and return `full` to the user.
func generate(prefix string) (id string, secretBytes, hash []byte, full string, err error) {
	idRaw := make([]byte, 13) // 13 * 8 = 104 bits → 21 base32 chars, we take 20
	secretRaw := make([]byte, 13)
	if _, err = rand.Read(idRaw); err != nil {
		return "", nil, nil, "", err
	}
	if _, err = rand.Read(secretRaw); err != nil {
		return "", nil, nil, "", err
	}
	idPart := strings.ToLower(enc.EncodeToString(idRaw))[:idLen]
	secretPart := strings.ToLower(enc.EncodeToString(secretRaw))[:secretLen]
	id = prefix + idPart

	// We want to hash the DECODED secret, not the base32 text, so the
	// stored hash matches what Parse returns.
	secretBytes, err = enc.DecodeString(strings.ToUpper(secretPart))
	if err != nil {
		return "", nil, nil, "", fmt.Errorf("decode freshly-generated secret: %w", err)
	}
	h := sha256.Sum256(secretBytes)
	hash = h[:]
	full = id + "." + secretPart
	return
}

// Service ties the storage repos together and exposes the enrollment
// operations that handlers / agents call.
type Service struct {
	db *storage.DB
}

func New(db *storage.DB) *Service { return &Service{db: db} }

// IssuePATResult is what Mint returns. PlaintextToken is the only
// moment the plaintext exists in memory — the caller must return it in
// the HTTP response and then drop the reference.
type IssuePATResult struct {
	TokenID        string
	PlaintextToken string
	ExpiresAt      time.Time
	Token          *storage.PATToken
}

// MintPATInput captures all operator-supplied fields for a new PAT.
// Zero values get sensible defaults; MaxUses=0 is coerced to 1.
type MintPATInput struct {
	ProjectID        string
	IssuedByUser     string
	Description      string
	TTL              time.Duration
	MaxUses          int
	BindingMachineID string
	BindingHostAlias string
}

func (s *Service) MintPAT(ctx context.Context, in MintPATInput) (*IssuePATResult, error) {
	if in.ProjectID == "" || in.IssuedByUser == "" {
		return nil, errors.New("enrollment: project_id and issued_by_user required")
	}
	ttl := in.TTL
	if ttl <= 0 {
		ttl = DefaultPATTTL
	}
	maxUses := in.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}

	id, _, hash, full, err := generate(PATPrefix)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tok := &storage.PATToken{
		TokenID:          id,
		SecretHash:       hash,
		ProjectID:        in.ProjectID,
		IssuedByUser:     in.IssuedByUser,
		IssuedAt:         now,
		ExpiresAt:        now.Add(ttl),
		MaxUses:          maxUses,
		BindingMachineID: in.BindingMachineID,
		BindingHostAlias: in.BindingHostAlias,
		Description:      in.Description,
	}
	if err := s.db.PATTokens().Create(ctx, tok); err != nil {
		return nil, fmt.Errorf("enrollment: create PAT: %w", err)
	}
	return &IssuePATResult{
		TokenID:        id,
		PlaintextToken: full,
		ExpiresAt:      tok.ExpiresAt,
		Token:          tok,
	}, nil
}

// RevokePAT marks a PAT revoked and records the action in
// admin_audit_log. Returns ErrNotFound if the id doesn't match any row.
func (s *Service) RevokePAT(ctx context.Context, tokenID, actorUser, reason string) error {
	if err := s.db.PATTokens().Revoke(ctx, tokenID, actorUser, reason, time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

// RedeemResult is returned to the agent-facing handshake after a
// successful enrollment exchange. The SessionPlaintext is the ONLY
// moment the new session token exists in memory — the caller returns
// it in AgentEnrollResponse and drops the reference immediately.
type RedeemResult struct {
	AgentID          string
	SessionID        string
	SessionPlaintext string
	SessionExpiresAt time.Time
	Outcome          string // "success" always on nil error; diagnostic string otherwise
}

// RedeemContext is the per-request metadata the enrollment code needs
// to record audit events. ClientIP is the agent's apparent address.
type RedeemContext struct {
	ClientIP  string
	MachineID string
	Hostname  string
}

// RedeemPAT verifies a PAT string, atomically consumes one use, creates
// an agent_id + session for it, and records audit events. The result's
// Outcome is always populated even on the nil-error path so callers can
// log a consistent classification.
func (s *Service) RedeemPAT(ctx context.Context, raw string, rctx RedeemContext) (*RedeemResult, error) {
	parsed, parseErr := Parse(raw)
	if parseErr != nil || parsed.Kind != KindPAT {
		s.logRedemption(ctx, "", rctx, "", "malformed", "")
		return &RedeemResult{Outcome: "malformed"}, ErrMalformed
	}

	tok, outcome, err := s.db.PATTokens().TryConsume(ctx,
		parsed.ID, parsed.Secret, rctx.MachineID, time.Now().UTC())
	if err != nil {
		s.logRedemption(ctx, parsed.ID, rctx, "", "error", err.Error())
		return &RedeemResult{Outcome: "error"}, err
	}
	if outcome != "success" {
		s.logRedemption(ctx, parsed.ID, rctx, "", outcome, "")
		return &RedeemResult{Outcome: outcome}, nil
	}

	// PAT accepted — mint a brand-new agent_id and initial session.
	agentID := "agent-" + uuid.NewString()
	sess, plaintext, err := s.issueSession(ctx, agentID, tok.ProjectID, "enroll", rctx.MachineID)
	if err != nil {
		s.logRedemption(ctx, parsed.ID, rctx, agentID, "error", err.Error())
		return &RedeemResult{Outcome: "error"}, err
	}
	s.logRedemption(ctx, parsed.ID, rctx, agentID, "success", "")

	return &RedeemResult{
		AgentID:          agentID,
		SessionID:        sess.SessionID,
		SessionPlaintext: plaintext,
		SessionExpiresAt: sess.ExpiresAt,
		Outcome:          "success",
	}, nil
}

// RedeemSession verifies a session token and rotates it, producing a
// fresh session token for the agent to persist. This is the reconnect
// path — it does NOT touch PATs or create a new agent_id.
func (s *Service) RedeemSession(ctx context.Context, raw string, rctx RedeemContext) (*RedeemResult, error) {
	parsed, parseErr := Parse(raw)
	if parseErr != nil || parsed.Kind != KindSession {
		return &RedeemResult{Outcome: "malformed"}, ErrMalformed
	}

	current, err := s.db.AgentSessions().GetBySessionID(ctx, parsed.ID)
	if errors.Is(err, storage.ErrNotFound) {
		return &RedeemResult{Outcome: "unknown_session"}, nil
	}
	if err != nil {
		return &RedeemResult{Outcome: "error"}, err
	}
	if !current.IsActive(time.Now()) {
		return &RedeemResult{Outcome: "session_inactive"}, nil
	}
	// Constant-time compare to protect against timing side channels, even
	// though we've already indexed the session lookup by id.
	wantHash := sha256.Sum256(parsed.Secret)
	if !bytesEqualCT(wantHash[:], current.SessionTokenHash) {
		return &RedeemResult{Outcome: "invalid_secret"}, nil
	}

	// Rotate.
	next, plaintext, err := s.rotateSession(ctx, current, rctx.MachineID)
	if err != nil {
		return &RedeemResult{Outcome: "error"}, err
	}
	return &RedeemResult{
		AgentID:          current.AgentID,
		SessionID:        next.SessionID,
		SessionPlaintext: plaintext,
		SessionExpiresAt: next.ExpiresAt,
		Outcome:          "success",
	}, nil
}

// issueSession creates a brand-new active session for an agent. Used on
// first enrollment.
func (s *Service) issueSession(ctx context.Context, agentID, projectID, reason, machineID string) (*storage.AgentSession, string, error) {
	id, _, hash, full, err := generate(SessionPrefix)
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	sess := &storage.AgentSession{
		SessionID:        id,
		AgentID:          agentID,
		ProjectID:        projectID,
		SessionTokenHash: hash,
		IssuedAt:         now,
		IssuedReason:     reason,
		ExpiresAt:        now.Add(DefaultSessionTTL),
		MachineID:        machineID,
	}
	if err := s.db.AgentSessions().InsertActive(ctx, sess); err != nil {
		return nil, "", err
	}
	return sess, full, nil
}

// rotateSession creates a new generation and marks the current one as
// rotated. Runs inside a single transaction via storage.AgentSessions.RotateTo.
func (s *Service) rotateSession(ctx context.Context, current *storage.AgentSession, machineID string) (*storage.AgentSession, string, error) {
	id, _, hash, full, err := generate(SessionPrefix)
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	next := &storage.AgentSession{
		SessionID:        id,
		AgentID:          current.AgentID,
		ProjectID:        current.ProjectID,
		SessionTokenHash: hash,
		IssuedAt:         now,
		IssuedReason:     "rotation",
		ExpiresAt:        now.Add(DefaultSessionTTL),
		MachineID:        machineID,
	}
	if err := s.db.AgentSessions().RotateTo(ctx, current.SessionID, next, now); err != nil {
		return nil, "", err
	}
	return next, full, nil
}

// logRedemption writes a row into pat_redemption_events. Swallows any
// log-write error — failing the enrollment flow over a logging hiccup
// would be worse than the missing log line.
func (s *Service) logRedemption(ctx context.Context, tokenID string, rctx RedeemContext, agentID, outcome, errDetail string) {
	_ = s.db.PATRedemptionEvents().Record(ctx, &storage.PATRedemptionEvent{
		At:          time.Now().UTC(),
		TokenID:     tokenID,
		ClientIP:    rctx.ClientIP,
		MachineID:   rctx.MachineID,
		Hostname:    rctx.Hostname,
		AgentID:     agentID,
		Outcome:     outcome,
		ErrorDetail: errDetail,
	})
}

// bytesEqualCT is subtle.ConstantTimeCompare wrapped so tests can stub.
func bytesEqualCT(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
