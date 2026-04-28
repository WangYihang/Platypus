// Package enrollment handles the PAT credential lifecycle: minting new
// PATs and redeeming them. Post-enrollment identity is carried by a
// project-CA-signed client certificate (see internal/pki and
// handler_enroll_v2.go), not by rotating session tokens — the
// server-side session-token store has been removed.
//
// The package deliberately sits above internal/storage but below
// internal/api and internal/core so it can be called from either the
// admin REST layer (minting) or the v2 enrollment handler (redeeming)
// without creating a cycle.
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

	"github.com/WangYihang/Platypus/internal/activity"
	"github.com/WangYihang/Platypus/internal/storage"
)

const (
	// EnrollmentTokenPrefix tags tokens issued as one-shot provisioning credentials.
	// Visible in logs / git / Slack so secret scanners can match it the
	// same way they do GitHub's `ghp_`.
	EnrollmentTokenPrefix = "plt_"

	// Token shape: "<prefix><id>.<secret>". The id half is the primary
	// key in enrollment_tokens (indexed); the secret half is what we hash and
	// compare against the stored SHA-256 digest.
	idLen     = 20
	secretLen = 20

	// DefaultEnrollmentTokenTTL is applied when an issue request leaves ttl_seconds
	// unset. Short enough that an accidentally-leaked token burns out
	// quickly; long enough to survive manual copy-paste flows.
	DefaultEnrollmentTokenTTL = 1 * time.Hour
)

// enc is unpadded lowercase base32 — URL-safe, case-insensitive, and
// (critically for test stability and log grep) free of confusable chars
// like I/l/0/O once decoded to lowercase.
var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// CredentialKind classifies which credential shape a caller is
// presenting. Retained as a tag on ParsedCredential for symmetry with
// other credential parsers in the codebase, but only PATs are live.
type CredentialKind int

const (
	KindUnknown CredentialKind = iota
	KindEnrollmentToken
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

// Parse splits a presented PAT credential. It does NOT validate the
// credential against storage — that's RedeemEnrollmentToken's job.
func Parse(raw string) (*ParsedCredential, error) {
	if !strings.HasPrefix(raw, EnrollmentTokenPrefix) {
		return nil, ErrMalformed
	}
	rest := strings.TrimPrefix(raw, EnrollmentTokenPrefix)
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrMalformed
	}
	secret, err := enc.DecodeString(strings.ToUpper(parts[1]))
	if err != nil {
		return nil, ErrMalformed
	}
	return &ParsedCredential{
		Kind:   KindEnrollmentToken,
		ID:     EnrollmentTokenPrefix + parts[0],
		Secret: secret,
	}, nil
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
	// pki is optional — when set, enrollment responses include a
	// freshly-signed agent identity cert. When nil, enrollment still
	// works using PSK + session_token only (Phase 1–3 behaviour).
	pki PKIIssuer
	// settings is optional — when set, MintInstallArtifact consults
	// it for the default PAT TTL so admins can tune the default from
	// the Web UI without a server restart. When nil, falls back to
	// the DefaultEnrollmentTokenTTL constant.
	settings SettingsProvider
}

// SettingsProvider exposes the subset of settings.Registry the
// enrollment package consults at mint time. Kept as an interface so
// enrollment has no compile-time dependency on internal/settings.
type SettingsProvider interface {
	PATDefaultTTL() time.Duration
}

// PKIIssuer is the subset of internal/pki.Service that enrollment
// depends on. Declared as an interface so the enrollment package
// doesn't import internal/pki directly — that keeps the dep graph
// linear (pki → enrollment → nothing pki-aware).
type PKIIssuer interface {
	// IssueForAgent issues a cert binding (agent_id, pubkey). The
	// caller passes a raw Ed25519 pubkey; the implementation handles
	// CA ensure + serial alloc + signing atomically. Returns empty
	// strings with nil error if PKI isn't configured on the server;
	// returns non-nil error for hard failures (KEK missing, etc).
	IssueForAgent(ctx context.Context, projectID, agentID string, pubkey []byte, reason string) (certPEM, caPEM string, err error)
}

func New(db *storage.DB) *Service { return &Service{db: db} }

// WithPKI attaches a PKI issuer. Call this during bootstrap after
// both services are constructed. Calling it more than once replaces
// the issuer — useful for tests.
func (s *Service) WithPKI(p PKIIssuer) *Service {
	s.pki = p
	return s
}

// WithSettings attaches a settings provider used for default-TTL
// lookups on mint. Idempotent.
func (s *Service) WithSettings(sp SettingsProvider) *Service {
	s.settings = sp
	return s
}

// IssueEnrollmentTokenResult is what Mint returns. PlaintextToken is the only
// moment the plaintext exists in memory — the caller must return it in
// the HTTP response and then drop the reference.
type IssueEnrollmentTokenResult struct {
	TokenID        string
	PlaintextToken string
	ExpiresAt      time.Time
	Token          *storage.EnrollmentToken
}

// MintEnrollmentTokenInput captures all operator-supplied fields for a new PAT.
// Zero values get sensible defaults; MaxUses=0 is coerced to 1.
type MintEnrollmentTokenInput struct {
	ProjectID        string
	IssuedByUser     string
	Description      string
	TTL              time.Duration
	MaxUses          int
	BindingMachineID string
	BindingHostAlias string
}

func (s *Service) MintEnrollmentToken(ctx context.Context, in MintEnrollmentTokenInput) (*IssueEnrollmentTokenResult, error) {
	if in.ProjectID == "" || in.IssuedByUser == "" {
		return nil, errors.New("enrollment: project_id and issued_by_user required")
	}
	ttl := in.TTL
	if ttl <= 0 {
		ttl = DefaultEnrollmentTokenTTL
	}
	maxUses := in.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}

	id, _, hash, full, err := generate(EnrollmentTokenPrefix)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tok := &storage.EnrollmentToken{
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
	if err := s.db.EnrollmentTokens().Create(ctx, tok); err != nil {
		return nil, fmt.Errorf("enrollment: create PAT: %w", err)
	}
	return &IssueEnrollmentTokenResult{
		TokenID:        id,
		PlaintextToken: full,
		ExpiresAt:      tok.ExpiresAt,
		Token:          tok,
	}, nil
}

// RevokeEnrollmentToken marks a PAT revoked and records the action in
// admin_audit_log. Returns ErrNotFound if the id doesn't match any row.
func (s *Service) RevokeEnrollmentToken(ctx context.Context, tokenID, actorUser, reason string) error {
	if err := s.db.EnrollmentTokens().Revoke(ctx, tokenID, actorUser, reason, time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

// RedeemResult is returned to the v2 enrollment handler after a
// successful PAT redemption. AgentID / ProjectID are the bindings the
// caller stamps into the final client certificate; CertPEM / CAPem
// are populated by the legacy IssueForAgent path when the caller
// supplied an Ed25519 pubkey. Today's v2 handler leaves pubkey empty
// and issues the cert itself from a CSR — CertPEM will be "" on that
// path, which is fine.
type RedeemResult struct {
	AgentID   string
	ProjectID string
	Outcome   string // "success" always on nil error; diagnostic string otherwise

	CertPEM string
	CAPem   string
}

// RedeemContext is the per-request metadata the enrollment code needs
// to record audit events. ClientIP is the agent's apparent address.
type RedeemContext struct {
	ClientIP  string
	MachineID string
	Hostname  string

	// AgentPubKey is the Ed25519 public key the agent advertised in
	// AgentEnrollRequest.pubkey. When non-empty and a PKI issuer is
	// attached, enrollment mints a leaf cert binding this key and
	// returns it in CertPEM. Empty → no cert.
	AgentPubKey []byte
}

// RedeemEnrollmentToken verifies a PAT string, atomically consumes one use, mints
// a fresh agent_id, and records audit events. Post-v2, agent identity
// is carried by the client certificate the caller issues from the
// returned (AgentID, ProjectID) — no server-side session token is
// created. The result's Outcome is always populated even on the
// nil-error path so callers can log a consistent classification.
func (s *Service) RedeemEnrollmentToken(ctx context.Context, raw string, rctx RedeemContext) (*RedeemResult, error) {
	parsed, parseErr := Parse(raw)
	if parseErr != nil || parsed.Kind != KindEnrollmentToken {
		s.logRedemption(ctx, "", rctx, "", "malformed", "")
		return &RedeemResult{Outcome: "malformed"}, ErrMalformed
	}

	tok, outcome, err := s.db.EnrollmentTokens().TryConsume(ctx,
		parsed.ID, parsed.Secret, rctx.MachineID, time.Now().UTC())
	if err != nil {
		s.logRedemption(ctx, parsed.ID, rctx, "", "error", err.Error())
		return &RedeemResult{Outcome: "error"}, err
	}
	if outcome != "success" {
		s.logRedemption(ctx, parsed.ID, rctx, "", outcome, "")
		return &RedeemResult{Outcome: outcome}, nil
	}

	// PAT accepted — mint a brand-new agent_id. The caller (v2
	// enrollment handler) signs the CSR and returns the cert; we don't
	// need any server-side state bound to agent_id here.
	agentID := "agent-" + uuid.NewString()
	s.logRedemption(ctx, parsed.ID, rctx, agentID, "success", "")

	certPEM, caPEM := s.maybeIssueCert(ctx, tok.ProjectID, agentID, rctx.AgentPubKey, "enroll")

	return &RedeemResult{
		AgentID:   agentID,
		ProjectID: tok.ProjectID,
		Outcome:   "success",
		CertPEM:   certPEM,
		CAPem:     caPEM,
	}, nil
}

// maybeIssueCert delegates to the attached PKI issuer when both PKI
// is configured AND the caller supplied a pubkey. It's the legacy
// "issue cert from raw pubkey" code path; the v2 enrollment handler
// doesn't use it — it calls IssueAgentLeafFromCSR directly so the
// cert binds to a key the server has actually verified against a
// CSR. Kept here because MintEnrollmentToken callers that predate the CSR path
// still rely on it returning empty-string on the common "no PKI /
// no pubkey" combination.
func (s *Service) maybeIssueCert(ctx context.Context, projectID, agentID string, pubkey []byte, reason string) (string, string) {
	if s.pki == nil || len(pubkey) == 0 {
		return "", ""
	}
	certPEM, caPEM, err := s.pki.IssueForAgent(ctx, projectID, agentID, pubkey, reason)
	if err != nil {
		// The maybeIssueCert contract: log-and-return-empty. The
		// RedeemEnrollmentToken caller still emits the "success" enrollment event;
		// the CA failure surfaces in the PKI handler's own audit log.
		return "", ""
	}
	return certPEM, caPEM
}

// logRedemption writes a PAT / session redemption attempt into the
// unified activities log. Every attempt — success or otherwise — lands
// here so scanning against bogus token_ids stays visible.
//
// Outcomes map onto the audit outcome tri-state: "success" stays as-is,
// "error" is a server fault, and everything else (invalid_secret,
// expired, revoked, max_uses_reached, …) is classified as "denied"
// because it's a rejected attempt.
func (s *Service) logRedemption(ctx context.Context, tokenID string, rctx RedeemContext, agentID, outcome, errDetail string) {
	action := "pat.redeem"
	auditOutcome := storage.OutcomeDenied
	switch outcome {
	case "success":
		auditOutcome = storage.OutcomeSuccess
	case "error":
		action = "pat.redeem_failed"
		auditOutcome = storage.OutcomeError
	default:
		action = "pat.redeem_failed"
	}

	meta := map[string]any{
		"reason":     outcome,
		"machine_id": rctx.MachineID,
		"hostname":   rctx.Hostname,
	}
	if agentID != "" {
		meta["agent_id"] = agentID
	}

	// Resolve project id from the PAT row when known; the redemption
	// may fail before we have a token, in which case the event is
	// truly global (a scanning attempt that doesn't belong to any
	// project).
	var projectID *string
	if tokenID != "" {
		if tok, err := s.db.EnrollmentTokens().Get(ctx, tokenID); err == nil {
			pid := tok.ProjectID
			projectID = &pid
		}
	}

	in := activity.Input{
		ProjectID:    projectID,
		ActorType:    storage.ActorTypeAgent,
		ActorIP:      rctx.ClientIP,
		ActorTokenID: tokenID,
		Category:     storage.CategoryAuth,
		Action:       action,
		TargetType:   "pat_token",
		TargetID:     tokenID,
		TargetLabel:  tokenID,
		Outcome:      auditOutcome,
		Error:        errDetail,
		SessionID:    "",
		Meta:         meta,
	}
	if agentID != "" {
		in.ActorUser = agentID // agent acts as itself once enrolled
	}
	activity.RecordWithContext(ctx, in)
}

// bytesEqualCT is subtle.ConstantTimeCompare wrapped so tests can stub.
// Used by install.go when verifying install-token secrets.
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
