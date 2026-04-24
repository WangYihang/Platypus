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
// ProjectCA is used for verifying the server's TLS chain on the
// initial enroll POST (and on the subsequent link dial once we've
// loaded our own identity). It's normally built from
// LoadProjectCA(os.Getenv("PLATYPUS_PROJECT_CA")) at startup.
type BootstrapV2Options struct {
	IdentityDir string
	ServerURL   string // wss://host:port (link endpoint path is appended)
	EnrollURL   string // https://host:port (enroll endpoint path is appended)

	// Enroll-only fields.
	PAT          string
	ProjectCA    *x509.CertPool
	Hostname     string
	MachineID    string
	AgentVersion string
}

// BootstrapV2 returns a live link.Session, running the full agent-
// side bring-up:
//
//  1. Try LoadIdentity(IdentityDir). If found, skip to step 4.
//  2. Require PAT + EnrollURL + ProjectCA; call Enroll.
//  3. SaveIdentity from the enrollment response.
//  4. BuildDialerTLSConfig from the identity.
//  5. Dial ServerURL (as wss://) with the TLS config.
//
// The caller owns the returned *link.Session and must Close it.
func BootstrapV2(ctx context.Context, opts BootstrapV2Options) (*link.Session, error) {
	if opts.IdentityDir == "" {
		opts.IdentityDir = ResolveIdentityDir("")
	}
	if opts.ServerURL == "" {
		return nil, errors.New("agent: BootstrapV2: ServerURL required")
	}

	id, err := LoadIdentity(opts.IdentityDir)
	if errors.Is(err, ErrIdentityNotFound) {
		id, err = enrollAndPersist(ctx, opts)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, fmt.Errorf("agent: BootstrapV2 load identity: %w", err)
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

// enrollAndPersist handles the "no identity on disk" branch: POST
// the PAT + CSR to the enroll endpoint, then save the cert + key +
// CA locally so subsequent boots skip this step.
func enrollAndPersist(ctx context.Context, opts BootstrapV2Options) (*Identity, error) {
	if opts.PAT == "" {
		return nil, errors.New("agent: BootstrapV2: no identity on disk and no PAT; cannot enroll")
	}
	if opts.EnrollURL == "" {
		return nil, errors.New("agent: BootstrapV2: EnrollURL required when enrolling")
	}
	res, err := Enroll(ctx, EnrollOptions{
		ServerURL:    opts.EnrollURL,
		PAT:          opts.PAT,
		Hostname:     opts.Hostname,
		MachineID:    opts.MachineID,
		AgentVersion: opts.AgentVersion,
		ProjectCA:    opts.ProjectCA,
	})
	if err != nil {
		return nil, err
	}
	if err := SaveIdentity(opts.IdentityDir, res.PrivateKey, res.Identity.CertPEM, res.Identity.CAPEM); err != nil {
		return nil, fmt.Errorf("agent: BootstrapV2 persist identity: %w", err)
	}
	return &res.Identity, nil
}
