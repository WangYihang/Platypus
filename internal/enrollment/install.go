package enrollment

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/storage"
)

const (
	// InstallPrefix tags one-shot download handles issued by the admin
	// REST surface. Visible in curl commands / logs so secret scanners
	// can pick them up.
	InstallPrefix = "dl_"

	// DefaultInstallTTL applies when the admin leaves it unset on a
	// new install artifact. 5 minutes is long enough to paste into a
	// terminal, short enough that a leaked URL self-heals fast.
	DefaultInstallTTL = 5 * time.Minute
)

// InstallArtifact is what MintInstallArtifact returns to the admin
// handler. The caller must surface PlaintextDownloadToken (i.e. the
// complete `dl_<id>.<secret>` string) to the user in the HTTP response
// and then drop the reference — the plaintext never appears again.
//
// The PAT itself is NOT materialised here: it only springs into
// existence when the download URL is actually curled. That way an
// unused install link carries no embedded credential at all, so a
// leaked URL that's never consumed has zero blast radius.
type InstallArtifact struct {
	DownloadID             string
	PlaintextDownloadToken string
	ExpiresAt              time.Time
	ServerEndpoint         string
	TargetOS               string
	TargetArch             string
}

// MintInstallArtifactInput groups the admin-supplied knobs. ServerEndpoint
// is required; everything else defaults sensibly.
type MintInstallArtifactInput struct {
	ProjectID           string
	IssuedByUser        string
	ServerEndpoint      string        // "host:port" that the agent should dial
	TargetOS            string        // optional: "linux", "darwin", "windows"
	TargetArch          string        // optional: "amd64", "arm64"
	TTL                 time.Duration // default DefaultInstallTTL
	PATTTL              time.Duration // propagated into the minted PAT
	PATBindingMachineID string
	PATDescription      string
	// AutoApprove makes the host that redeems this install link
	// enroll directly to `approved`, skipping the admin Approve step.
	// Use for automation flows (Ansible, CI, cloud-init) where there
	// is no human-in-the-loop. Default false: a leaked install link
	// can still create a `pending` host but can't reach the agent
	// runtime until an admin clicks Approve.
	AutoApprove bool
}

// MintInstallArtifact inserts a new install_download_tokens row and
// returns the plaintext handle so the REST layer can build the curl
// command. Does NOT mint the PAT yet — see ConsumeInstallDownload.
func (s *Service) MintInstallArtifact(ctx context.Context, in MintInstallArtifactInput) (*InstallArtifact, error) {
	if in.ProjectID == "" || in.IssuedByUser == "" {
		return nil, errors.New("enrollment: project_id and issued_by_user required")
	}
	if in.ServerEndpoint == "" {
		return nil, errors.New("enrollment: server_endpoint required")
	}

	ttl := in.TTL
	if ttl <= 0 {
		ttl = DefaultInstallTTL
	}
	patTTL := in.PATTTL
	if patTTL <= 0 {
		if s.settings != nil {
			if d := s.settings.PATDefaultTTL(); d > 0 {
				patTTL = d
			}
		}
	}
	if patTTL <= 0 {
		patTTL = DefaultEnrollmentTokenTTL
	}

	id, _, hash, full, err := generate(InstallPrefix)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tok := &storage.InstallDownloadToken{
		DownloadID:          id,
		SecretHash:          hash,
		ProjectID:           in.ProjectID,
		IssuedByUser:        in.IssuedByUser,
		IssuedAt:            now,
		ExpiresAt:           now.Add(ttl),
		TargetOS:            in.TargetOS,
		TargetArch:          in.TargetArch,
		ServerEndpoint:      in.ServerEndpoint,
		PATTTLSeconds:       int(patTTL / time.Second),
		PATBindingMachineID: in.PATBindingMachineID,
		PATDescription:      in.PATDescription,
		AutoApprove:         in.AutoApprove,
	}
	if err := s.db.InstallDownloadTokens().Create(ctx, tok); err != nil {
		return nil, fmt.Errorf("enrollment: create install download token: %w", err)
	}
	return &InstallArtifact{
		DownloadID:             id,
		PlaintextDownloadToken: full,
		ExpiresAt:              tok.ExpiresAt,
		ServerEndpoint:         in.ServerEndpoint,
		TargetOS:               in.TargetOS,
		TargetArch:             in.TargetArch,
	}, nil
}

// ConsumeResult is what ConsumeInstallDownload returns to the distributor.
// PATPlaintext is the freshly-minted PAT that should be embedded into
// the rendered shell script. Outcome is always set for audit logging.
//
// ProjectID + ProjectCAPEM are populated on success when the project
// has an initialised CA. The distributor embeds the CA PEM in the
// install script so the agent can verify the server's TLS chain on
// subsequent connections without relying on TOFU or InsecureSkipVerify.
// Both empty means PKI isn't configured; the agent falls back to
// skip-verify (legacy behaviour).
type ConsumeResult struct {
	Outcome        string // "success" / "unknown_id" / "invalid_secret" / "expired" / "revoked" / "already_consumed" / "malformed"
	DownloadID     string
	ServerEndpoint string
	TargetOS       string
	TargetArch     string
	PATTokenID     string
	PATPlaintext   string
	PATExpiresAt   time.Time
	ProjectID      string
	ProjectCAPEM   string
}

// ConsumeContext carries request metadata used for audit rows.
type ConsumeContext struct {
	ClientIP string
	ClientUA string
}

// ConsumeInstallDownload validates a `dl_<id>.<secret>` credential,
// mints a fresh PAT, and atomically marks the install token consumed.
// The order-of-operations matters:
//
//  1. Validate the install token in a read (fast reject on unknown /
//     invalid-secret / expired / revoked / already-consumed).
//  2. Mint the PAT (this is the only side-effect we commit
//     unconditionally on reach).
//  3. Call TryConsume which atomically records the PAT linkage. If
//     another curl beats us here the second caller sees
//     "already_consumed" — the extra PAT we minted stays revokable
//     from the admin UI (it's an orphan, but append-only storage
//     policy forbids deleting it).
//
// An alternative would be to bring PAT minting inside the same
// transaction, but storage.PATTokens.Create runs its own writes and
// the resulting nested-tx logic would be much harder to reason about.
// The orphan-PAT risk is bounded (only occurs on true race) and
// observable (idx_pat_unrevoked shows it).
func (s *Service) ConsumeInstallDownload(ctx context.Context, raw string, cctx ConsumeContext) (*ConsumeResult, error) {
	id, secret, err := parseInstallToken(raw)
	if err != nil {
		s.logInstallEvent(ctx, "", cctx, "", "malformed", "")
		return &ConsumeResult{Outcome: "malformed"}, ErrMalformed
	}

	// Pre-check: is the install token even plausibly live? Cheap read
	// avoids minting a throwaway PAT against an invalid request.
	tok, err := s.db.InstallDownloadTokens().Get(ctx, id)
	if err != nil {
		s.logInstallEvent(ctx, id, cctx, "", "unknown_id", "")
		return &ConsumeResult{Outcome: "unknown_id"}, nil
	}

	// Check secret first (constant-time). If it's wrong we log and bail
	// without minting the PAT.
	hash := sha256.Sum256(secret)
	if !bytesEqualCT(hash[:], tok.SecretHash) {
		s.logInstallEvent(ctx, id, cctx, "", "invalid_secret", "")
		return &ConsumeResult{Outcome: "invalid_secret"}, nil
	}

	// Now mint the PAT. It inherits TTL + binding + auto_approve
	// from the install token so the consume-time PAT carries the same
	// pre-authorization decision the admin made at mint time.
	patRes, err := s.MintEnrollmentToken(ctx, MintEnrollmentTokenInput{
		ProjectID:        tok.ProjectID,
		IssuedByUser:     tok.IssuedByUser,
		Description:      patDescFor(tok),
		TTL:              time.Duration(tok.PATTTLSeconds) * time.Second,
		MaxUses:          1,
		BindingMachineID: tok.PATBindingMachineID,
		AutoApprove:      tok.AutoApprove,
	})
	if err != nil {
		s.logInstallEvent(ctx, id, cctx, "", "error", err.Error())
		return &ConsumeResult{Outcome: "error"}, err
	}

	// Atomic consume. On race / expiry / revoked we fall back to the
	// TryConsume-reported outcome; the PAT we just minted remains but
	// is revokable via the normal admin flow.
	_, outcome, err := s.db.InstallDownloadTokens().TryConsume(ctx,
		id, secret, cctx.ClientIP, cctx.ClientUA,
		patRes.TokenID, time.Now().UTC())
	if err != nil {
		s.logInstallEvent(ctx, id, cctx, patRes.TokenID, "error", err.Error())
		return &ConsumeResult{Outcome: "error"}, err
	}
	s.logInstallEvent(ctx, id, cctx, patRes.TokenID, outcome, "")
	if outcome != "success" {
		// Best-effort: proactively revoke the orphan PAT so it can never
		// be used. If revoke fails we log and move on — the admin can
		// still see it in ListByProject and revoke manually.
		_ = s.RevokeEnrollmentToken(ctx, patRes.TokenID, tok.IssuedByUser,
			"install download race: "+outcome)
		return &ConsumeResult{Outcome: outcome}, nil
	}

	// Best-effort CA lookup. Missing is fine — agents built before
	// the v2 enroll flow ignore PLATYPUS_PROJECT_CA anyway.
	var caPEM string
	if ca, caErr := s.db.ProjectCA().Get(ctx, tok.ProjectID); caErr == nil {
		caPEM = ca.CertPEM
	}

	return &ConsumeResult{
		Outcome:        "success",
		DownloadID:     id,
		ServerEndpoint: tok.ServerEndpoint,
		TargetOS:       tok.TargetOS,
		TargetArch:     tok.TargetArch,
		PATTokenID:     patRes.TokenID,
		PATPlaintext:   patRes.PlaintextToken,
		PATExpiresAt:   patRes.ExpiresAt,
		ProjectID:      tok.ProjectID,
		ProjectCAPEM:   caPEM,
	}, nil
}

// RevokeInstallDownload marks the install token revoked. Records the
// action in admin_audit_log via the caller (the REST handler).
func (s *Service) RevokeInstallDownload(ctx context.Context, id, actor, reason string) error {
	return s.db.InstallDownloadTokens().Revoke(ctx, id, actor, reason, time.Now().UTC())
}

// --- internals -----------------------------------------------------------

// parseInstallToken splits a `dl_<id>.<secret>` handle. We don't reuse
// enrollment.Parse because that one dispatches on `plt_` / `sess_`;
// install tokens have their own lifecycle so they get their own parser.
func parseInstallToken(raw string) (id string, secret []byte, err error) {
	if !strings.HasPrefix(raw, InstallPrefix) {
		return "", nil, ErrMalformed
	}
	rest := strings.TrimPrefix(raw, InstallPrefix)
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", nil, ErrMalformed
	}
	decoded, err := enc.DecodeString(strings.ToUpper(parts[1]))
	if err != nil {
		return "", nil, ErrMalformed
	}
	return InstallPrefix + parts[0], decoded, nil
}

func patDescFor(tok *storage.InstallDownloadToken) string {
	if tok.PATDescription != "" {
		return tok.PATDescription
	}
	return "auto-minted via install download " + tok.DownloadID
}

func (s *Service) logInstallEvent(ctx context.Context, id string, cctx ConsumeContext, patID, outcome, errDetail string) {
	_ = s.db.InstallDownloadEvents().Record(ctx, &storage.InstallDownloadEvent{
		At:          time.Now().UTC(),
		DownloadID:  id,
		ClientIP:    cctx.ClientIP,
		ClientUA:    cctx.ClientUA,
		PATTokenID:  patID,
		Outcome:     outcome,
		ErrorDetail: errDetail,
	})
}
