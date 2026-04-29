package agent

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/WangYihang/Platypus/internal/link"
)

// BootstrapV2Options bundles everything BootstrapV2 might need for
// either of its two paths. IdentityDir and ServerURL are always
// required; the rest are only consulted on the "fresh install"
// branch.
//
// ProjectCAPEM (raw bytes) drives both per-CA layout dispatch (its
// fingerprint picks the on-disk subdirectory under IdentityDir) and
// is parsed into the trust pool used for the enroll POST. When the
// caller already has a parsed pool — same bytes, parsed once — it
// can hand it in via ProjectCA to skip the re-parse, but the bytes
// are always required so different CAs land in different subdirs.
//
// On a restart that doesn't re-run the install script,
// PLATYPUS_PROJECT_CA isn't set, so ProjectCAPEM is empty and
// BootstrapV2 falls back to the active-pointer file under
// IdentityDir to find the previously-enrolled subdirectory.
type BootstrapV2Options struct {
	IdentityDir string // root dir; per-CA subdir resolved internally
	ServerURL   string // wss://host:port (link endpoint path is appended)
	EnrollURL   string // https://host:port (enroll endpoint path is appended)

	// Enroll-only fields.
	PAT          string
	ProjectCAPEM []byte
	ProjectCA    *x509.CertPool
	Hostname     string
	MachineID    string

	// Build identity, sourced from pkg/version. Forwarded to the
	// server so the host row records exactly which binary is on
	// the box. All three are advisory; the server never gates
	// security decisions on them.
	BuildVersion string // semver
	Commit       string // short git SHA
	BuildDate    string // RFC3339

	// Wire-protocol version this binary speaks. Sourced from
	// internal/link.ProtocolVersion. The server compares against
	// its MinSupportedProtocolVersion at enroll time.
	ProtocolVersion uint32
}

// BootstrapV2 returns a live link.Session, running the full agent-
// side bring-up:
//
//  1. Migrate any legacy flat-layout identity into the per-CA subdir
//     scheme so old installs keep working without re-enrollment.
//  2. Resolve the active per-CA subdir: from ProjectCAPEM's
//     fingerprint when the install script set PLATYPUS_PROJECT_CA,
//     otherwise from the active-pointer file written on a previous
//     successful enrollment.
//  3. Try LoadIdentity(subdir). If found, skip to step 6.
//  4. Require PAT + EnrollURL + ProjectCA(PEM); call Enroll.
//  5. SaveIdentity into the subdir scoped to the new CA's fingerprint
//     (which may differ from step 2's subdir if Enroll surprised us
//     with a different CA — in that case the resolved subdir
//     follows the response, and the active pointer updates to match).
//  6. BuildDialerTLSConfig from the identity.
//  7. Dial ServerURL (as wss://) with the TLS config.
//
// The caller owns the returned *link.Session and must Close it.
func BootstrapV2(ctx context.Context, opts BootstrapV2Options) (*link.Session, error) {
	if opts.IdentityDir == "" {
		opts.IdentityDir = ResolveIdentityDir("")
	}
	if opts.ServerURL == "" {
		return nil, errors.New("agent: BootstrapV2: ServerURL required")
	}

	root := opts.IdentityDir
	if err := MigrateLegacyIdentity(root); err != nil {
		return nil, fmt.Errorf("agent: BootstrapV2 migrate legacy identity: %w", err)
	}

	id, err := loadOrEnroll(ctx, root, opts)
	if err != nil {
		return nil, err
	}

	// Best-effort: refresh the active pointer so a later restart
	// without PLATYPUS_PROJECT_CA still finds the right subdir. Stale
	// pointer is recoverable (next install-script run rewrites it),
	// so we don't fail the bootstrap on this.
	currentFP, fpErr := CAFingerprint(id.CAPEM)
	if fpErr == nil {
		_ = WriteActive(root, currentFP)
	}

	tlsCfg, err := BuildDialerTLSConfig(id)
	if err != nil {
		return nil, fmt.Errorf("agent: BootstrapV2 build TLS config: %w", err)
	}

	sess, err := link.Dial(ctx, link.DialOptions{
		URL:       opts.ServerURL,
		TLSConfig: tlsCfg,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: BootstrapV2 dial: %w", err)
	}
	return sess, nil
}

// loadOrEnroll resolves the per-CA subdir, either loading the
// previously-saved identity or kicking off a fresh enrollment, and
// returns a usable Identity. Three branches:
//
//  1. PLATYPUS_PROJECT_CA bytes were passed in (install script run).
//     They're authoritative — they pin the caller to "this is the
//     server you trust now" — so we look at the subdir keyed by
//     their fingerprint. Missing identity there means first contact
//     with this CA: enroll and save.
//  2. No env CA, but the active-pointer file points at a previously-
//     enrolled subdir. Restart-after-install path: just load it.
//  3. Neither env CA nor active pointer. The agent has never enrolled
//     and is missing the trust anchor for first enrollment — produce
//     a message that names the missing piece (PAT vs CA env var) so
//     operators see a useful hint instead of a bare "not found".
func loadOrEnroll(ctx context.Context, root string, opts BootstrapV2Options) (*Identity, error) {
	if len(opts.ProjectCAPEM) > 0 {
		fp, err := CAFingerprint(opts.ProjectCAPEM)
		if err != nil {
			return nil, fmt.Errorf("agent: BootstrapV2: ProjectCAPEM fingerprint: %w", err)
		}
		sub := IdentitySubdir(root, fp)
		id, err := LoadIdentity(sub)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, ErrIdentityNotFound) {
			return nil, fmt.Errorf("agent: BootstrapV2 load identity: %w", err)
		}
		return enrollAndPersist(ctx, opts)
	}

	active, err := ReadActive(root)
	if err != nil {
		return nil, fmt.Errorf("agent: BootstrapV2 read active: %w", err)
	}
	if active != "" {
		id, err := LoadIdentity(IdentitySubdir(root, active))
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, ErrIdentityNotFound) {
			return nil, fmt.Errorf("agent: BootstrapV2 load identity: %w", err)
		}
		// Pointer dangles (someone removed the subdir under us);
		// fall through to the first-run error path.
	}

	if opts.PAT == "" {
		return nil, errors.New("agent: BootstrapV2: no identity on disk and no PAT; cannot enroll")
	}
	return nil, errors.New("agent: BootstrapV2: no identity on disk and PLATYPUS_PROJECT_CA env var is empty; first enrollment needs the install script's project CA")
}

// enrollAndPersist handles the "no identity on disk" branch: POST
// the PAT + CSR to the enroll endpoint, then save the cert + key +
// CA into the subdir keyed by the *response's* CA fingerprint (which
// in the normal case matches the env CA the caller passed in, but
// stays correct if the server is in the middle of rotating keys).
func enrollAndPersist(ctx context.Context, opts BootstrapV2Options) (*Identity, error) {
	if opts.PAT == "" {
		return nil, errors.New("agent: BootstrapV2: no identity on disk and no PAT; cannot enroll")
	}
	if opts.EnrollURL == "" {
		return nil, errors.New("agent: BootstrapV2: EnrollURL required when enrolling")
	}
	// Capture a warm system snapshot once so the enrollment record
	// carries hardware / OS / network details. The server persists
	// these into the hosts row; future on-demand refreshes go via
	// the SysInfo RPC.
	snap := CollectSysInfo(ctx)
	res, err := Enroll(ctx, EnrollOptions{
		ServerURL:       opts.EnrollURL,
		PAT:             opts.PAT,
		Hostname:        opts.Hostname,
		MachineID:       opts.MachineID,
		BuildVersion:    opts.BuildVersion,
		Commit:          opts.Commit,
		BuildDate:       opts.BuildDate,
		ProtocolVersion: opts.ProtocolVersion,
		ProjectCA:       opts.ProjectCA,
		SysInfo:         snap,
	})
	if err != nil {
		return nil, err
	}
	// Subdir is keyed by the *response's* CA — almost always identical
	// to opts.ProjectCAPEM, but if the server is mid-rotation the
	// response is the authoritative trust anchor (it's the one the
	// dial pool gets built from), so let it pick the layout slot.
	fp, err := CAFingerprint(res.Identity.CAPEM)
	if err != nil {
		return nil, fmt.Errorf("agent: BootstrapV2 enroll: response CA fingerprint: %w", err)
	}
	sub := IdentitySubdir(opts.IdentityDir, fp)
	if err := SaveIdentity(sub, res.PrivateKey, res.Identity.CertPEM, res.Identity.CAPEM); err != nil {
		return nil, fmt.Errorf("agent: BootstrapV2 persist identity: %w", err)
	}
	if err := WriteActive(opts.IdentityDir, fp); err != nil {
		return nil, fmt.Errorf("agent: BootstrapV2 update active pointer: %w", err)
	}
	return &res.Identity, nil
}
