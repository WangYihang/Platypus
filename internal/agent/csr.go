package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// GenerateCSR mints a fresh Ed25519 keypair and wraps its public
// half in a PKCS#10 Certificate Signing Request. The server ignores
// the CSR's subject — the authoritative agent_id comes from PAT
// redemption on the /api/v1/agents/enroll endpoint — so we leave
// the subject empty.
//
// The returned PEM is ready to stuff straight into
// v2pb.EnrollRequest.csr_pem; the returned private key must be
// persisted alongside the cert the server issues, because the two
// are only useful together.
func GenerateCSR() ([]byte, ed25519.PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: ed25519 keygen: %w", err)
	}

	// An empty template is the intended usage: x509 fills in the
	// signature algorithm from the key type and everything else is
	// overwritten by the CA anyway.
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("agent: build CSR: %w", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
	return csrPEM, priv, nil
}
