package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"
)

// mintLeafPair mints a matched cert + PKCS#8-PEM Ed25519 key so
// the tests can stay independent of the agent package.
func mintLeafPair(t *testing.T, agentID, projectID string) (certPEM, keyPEM []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	agentURI, _ := url.Parse("platypus://agent/" + agentID)
	projectURI, _ := url.Parse("platypus://project/" + projectID)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(100),
		Subject:      pkix.Name{CommonName: agentID},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{agentURI, projectURI},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return
}

func TestLoadIdentityFromCert_HappyPath(t *testing.T) {
	certPEM, keyPEM := mintLeafPair(t, "agent-abc123", "proj-1")
	id, err := LoadIdentityFromCert(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadIdentityFromCert: %v", err)
	}
	if id.NodeID != "agent-abc123" {
		t.Fatalf("NodeID = %q; want agent-abc123", id.NodeID)
	}
	if len(id.PublicKey) != ed25519.PublicKeySize {
		t.Fatalf("PublicKey length = %d", len(id.PublicKey))
	}
	if len(id.PrivateKey) != ed25519.PrivateKeySize {
		t.Fatalf("PrivateKey length = %d", len(id.PrivateKey))
	}
	if string(id.CertPEM) != string(certPEM) {
		t.Fatal("CertPEM does not match input")
	}
	// NodeID must come from the cert SAN — prefix sanity check.
	if id.NodeID != "agent-abc123" {
		t.Fatalf("NodeID = %q, want agent-abc123", id.NodeID)
	}
}

// TestLoadIdentityFromCert_RejectsMismatchedKey feeds a cert issued
// for pubkey A together with the private key for pubkey B. The
// sanity check must catch this before we produce a broken Identity.
func TestLoadIdentityFromCert_RejectsMismatchedKey(t *testing.T) {
	certPEM, _ := mintLeafPair(t, "agent-1", "proj-1")
	_, unrelatedKeyPEM := mintLeafPair(t, "agent-2", "proj-1")

	if _, err := LoadIdentityFromCert(certPEM, unrelatedKeyPEM); err == nil {
		t.Fatal("expected error when cert and key come from different pairs")
	}
}

// TestLoadIdentityFromCert_RejectsCorruptKey confirms a non-PKCS#8
// or non-PEM key is rejected with a clear error.
func TestLoadIdentityFromCert_RejectsCorruptKey(t *testing.T) {
	certPEM, _ := mintLeafPair(t, "agent-1", "proj-1")
	if _, err := LoadIdentityFromCert(certPEM, []byte("garbage")); err == nil {
		t.Fatal("expected error on non-PEM key input")
	}
	badBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{0x00, 0x01}})
	if _, err := LoadIdentityFromCert(certPEM, badBlock); err == nil {
		t.Fatal("expected error on malformed PKCS#8 DER")
	}
}

// TestLoadIdentityFromCert_RejectsCertWithoutSAN rejects a cert
// that lacks the platypus://agent/<id> URI even if cert + key
// match each other — without the SAN there is no NodeID to bind.
func TestLoadIdentityFromCert_RejectsCertWithoutSAN(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(200),
		Subject:      pkix.Name{CommonName: "no-san"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, _ := x509.MarshalPKCS8PrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if _, err := LoadIdentityFromCert(certPEM, keyPEM); err == nil {
		t.Fatal("expected error on cert with no agent SAN")
	}
}
