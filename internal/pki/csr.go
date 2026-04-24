package pki

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
)

// CSRInput drives IssueAgentLeafFromCSR. The CSR itself carries the
// public key and proves possession of the matching private key; the
// server supplies the AgentID and ProjectID (which are trusted,
// having come from PAT redemption or from mTLS authentication on
// renewal).
type CSRInput struct {
	ProjectID    string
	AgentID      string
	CSRPEM       []byte
	Reason       string
	IssuedByUser string
}

// IssueAgentLeafFromCSR parses a PEM-encoded PKCS#10 CSR, validates
// that the embedded public key is Ed25519 and that the CSR signature
// is self-consistent, then delegates to IssueAgentCert with the
// extracted pubkey. The resulting leaf cert has the server-supplied
// AgentID in its CN and the URI SAN platypus://agent/<id>, plus
// platypus://project/<id>; mesh peers verify NodeID (which is
// derivable from the pubkey) against the cert-bound identity.
func (s *Service) IssueAgentLeafFromCSR(ctx context.Context, in CSRInput) (*IssueResult, error) {
	pub, err := parseCSRAndExtractPubkey(in.CSRPEM)
	if err != nil {
		return nil, err
	}
	return s.IssueAgentCert(ctx, IssueInput{
		ProjectID:    in.ProjectID,
		AgentID:      in.AgentID,
		AgentPubKey:  pub,
		Reason:       in.Reason,
		IssuedByUser: in.IssuedByUser,
	})
}

// parseCSRAndExtractPubkey decodes a PEM CSR, checks the signature,
// and returns the Ed25519 public key. Any other key algorithm is
// rejected — we only mint Ed25519 certs so a mismatched CSR is
// always an input error.
func parseCSRAndExtractPubkey(csrPEM []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, errors.New("pki: CSR PEM decode failed")
	}
	if block.Type != "CERTIFICATE REQUEST" && block.Type != "NEW CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("pki: unexpected PEM block type %q", block.Type)
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("pki: parse CSR: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("pki: CSR signature: %w", err)
	}
	pub, ok := csr.PublicKey.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("pki: CSR public key must be Ed25519")
	}
	if len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("pki: CSR Ed25519 key wrong length")
	}
	return pub, nil
}

// agentURISANs builds the URI SAN list stamped into leaf certs. The
// server trusts AgentID / ProjectID — they came from the enrollment
// flow — so these URIs are authoritative and callers (mesh handshake,
// AgentLink accept) can key off them without re-deriving.
func agentURISANs(agentID, projectID string) ([]*url.URL, error) {
	if agentID == "" {
		return nil, errors.New("pki: empty agent id")
	}
	if projectID == "" {
		return nil, errors.New("pki: empty project id")
	}
	agentURI, err := url.Parse("platypus://agent/" + url.PathEscape(agentID))
	if err != nil {
		return nil, err
	}
	projectURI, err := url.Parse("platypus://project/" + url.PathEscape(projectID))
	if err != nil {
		return nil, err
	}
	return []*url.URL{agentURI, projectURI}, nil
}
