package mesh

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// parseAgentLeafCert decodes a PEM bundle and returns the first
// CERTIFICATE block parsed as an x509.Certificate. The bundle may
// contain multiple blocks (leaf + CA chain); we take the leaf
// (first) and ignore the rest — chain verification is handled
// elsewhere against a trusted CA pool.
func parseAgentLeafCert(certPEM []byte) (*x509.Certificate, error) {
	if len(certPEM) == 0 {
		return nil, errors.New("mesh: empty cert PEM")
	}
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("mesh: no CERTIFICATE block in PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("mesh: parse leaf cert: %w", err)
	}
	return cert, nil
}

// agentIDFromCert pulls the "platypus://agent/<id>" URI SAN from a
// leaf cert. Mirrors internal/api.parseAgentSANs but returns only
// the agent id (mesh doesn't need the project id at this layer —
// project scoping is enforced by the trusted CA pool choice).
func agentIDFromCert(cert *x509.Certificate) (string, error) {
	for _, u := range cert.URIs {
		if u.Scheme != "platypus" || u.Host != "agent" {
			continue
		}
		id := strings.TrimPrefix(u.Path, "/")
		if id == "" {
			return "", errors.New("mesh: platypus://agent/ SAN has empty id")
		}
		return id, nil
	}
	return "", errors.New("mesh: cert missing platypus://agent/<id> URI SAN")
}

// ed25519PublicKeyFromCert extracts the leaf cert's Ed25519 public
// key. Platypus's PKI issues Ed25519 leaves exclusively; other key
// types are rejected.
func ed25519PublicKeyFromCert(cert *x509.Certificate) (ed25519.PublicKey, error) {
	pub, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("mesh: cert public key is %T, want ed25519.PublicKey", cert.PublicKey)
	}
	return pub, nil
}
