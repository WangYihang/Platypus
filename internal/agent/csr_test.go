package agent

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

// GenerateCSR is the agent-side counterpart to pki.IssueAgentLeafFromCSR:
// it mints a fresh Ed25519 keypair and returns the PEM-encoded PKCS#10
// CSR to send to /api/v1/agents/enroll, plus the private key so the
// agent can use it with the returned cert.

// Two calls must return two different keypairs. A deterministic
// generator would be a serious bug (identical agent_ids across hosts
// after enrollment).
func TestGenerateCSR_YieldsFreshKeypair(t *testing.T) {
	csr1, priv1, err := GenerateCSR()
	if err != nil {
		t.Fatalf("first GenerateCSR: %v", err)
	}
	csr2, priv2, err := GenerateCSR()
	if err != nil {
		t.Fatalf("second GenerateCSR: %v", err)
	}
	if len(priv1) != ed25519.PrivateKeySize || len(priv2) != ed25519.PrivateKeySize {
		t.Fatalf("privkey size: got %d and %d; want %d",
			len(priv1), len(priv2), ed25519.PrivateKeySize)
	}
	if priv1.Equal(priv2) {
		t.Fatal("two GenerateCSR calls produced identical private keys")
	}
	if len(csr1) == 0 || len(csr2) == 0 {
		t.Fatal("empty CSR PEM from GenerateCSR")
	}
}

// The returned PEM block is a CERTIFICATE REQUEST, decodes to a
// valid x509.CertificateRequest, is signed with Ed25519, and its
// embedded public key matches the private key we got back.
func TestGenerateCSR_ProducesValidPKCS10(t *testing.T) {
	csrPEM, priv, err := GenerateCSR()
	if err != nil {
		t.Fatalf("GenerateCSR: %v", err)
	}

	block, _ := pem.Decode(csrPEM)
	if block == nil {
		t.Fatal("pem.Decode: nil block")
	}
	if block.Type != "CERTIFICATE REQUEST" {
		t.Fatalf("block type = %q; want CERTIFICATE REQUEST", block.Type)
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificateRequest: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("CSR signature invalid: %v", err)
	}
	// Ed25519 in x509 reports its SignatureAlgorithm as
	// x509.PureEd25519.
	if csr.SignatureAlgorithm != x509.PureEd25519 {
		t.Fatalf("signature alg = %v; want PureEd25519", csr.SignatureAlgorithm)
	}
	pub, ok := csr.PublicKey.(ed25519.PublicKey)
	if !ok {
		t.Fatalf("CSR public key type = %T; want ed25519.PublicKey", csr.PublicKey)
	}
	if !priv.Public().(ed25519.PublicKey).Equal(pub) {
		t.Fatal("CSR public key does not match returned private key")
	}
}
