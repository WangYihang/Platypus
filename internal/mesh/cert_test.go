package mesh

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"
)

// mintTestLeafCert builds a self-signed Ed25519 leaf cert with the
// same URI SAN layout the Platypus PKI emits. Usable standalone in
// mesh tests without pulling in the full internal/pki setup.
func mintTestLeafCert(t *testing.T, agentID, projectID string) (certPEM []byte, priv ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	agentURI, _ := url.Parse("platypus://agent/" + agentID)
	projectURI, _ := url.Parse("platypus://project/" + projectID)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
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
	return certPEM, priv
}

func TestParseAgentLeafCert_HappyPath(t *testing.T) {
	certPEM, _ := mintTestLeafCert(t, "agent-abc", "proj-1")
	cert, err := parseAgentLeafCert(certPEM)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cert.Subject.CommonName != "agent-abc" {
		t.Fatalf("CN = %q; want agent-abc", cert.Subject.CommonName)
	}
}

func TestParseAgentLeafCert_RejectsNonPEM(t *testing.T) {
	if _, err := parseAgentLeafCert([]byte("not a pem")); err == nil {
		t.Fatal("expected parse to fail on non-PEM input")
	}
	if _, err := parseAgentLeafCert(nil); err == nil {
		t.Fatal("expected parse to fail on nil input")
	}
}

func TestParseAgentLeafCert_RejectsWrongBlockType(t *testing.T) {
	bogus := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{0x30, 0x00}})
	if _, err := parseAgentLeafCert(bogus); err == nil {
		t.Fatal("expected parse to fail when block type isn't CERTIFICATE")
	}
}

func TestAgentIDFromCert_HappyPath(t *testing.T) {
	certPEM, _ := mintTestLeafCert(t, "agent-xyz", "proj-2")
	cert, err := parseAgentLeafCert(certPEM)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	id, err := agentIDFromCert(cert)
	if err != nil {
		t.Fatalf("agentIDFromCert: %v", err)
	}
	if id != "agent-xyz" {
		t.Fatalf("id = %q; want agent-xyz", id)
	}
}

func TestAgentIDFromCert_RejectsMissingSAN(t *testing.T) {
	// Cert with no platypus:// URI SAN at all.
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "bare"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	cert, _ := x509.ParseCertificate(der)

	if _, err := agentIDFromCert(cert); err == nil {
		t.Fatal("expected error on cert without agent SAN")
	} else if !strings.Contains(err.Error(), "missing platypus://agent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentIDFromCert_RejectsEmptyID(t *testing.T) {
	u, _ := url.Parse("platypus://agent/")
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "empty-id"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{u},
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	cert, _ := x509.ParseCertificate(der)

	if _, err := agentIDFromCert(cert); err == nil {
		t.Fatal("expected error on empty-id SAN")
	}
}

func TestEd25519PublicKeyFromCert_HappyPath(t *testing.T) {
	certPEM, priv := mintTestLeafCert(t, "agent-k", "proj-1")
	cert, _ := parseAgentLeafCert(certPEM)

	got, err := ed25519PublicKeyFromCert(cert)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	want := priv.Public().(ed25519.PublicKey)
	if string(got) != string(want) {
		t.Fatal("extracted pubkey does not match cert's signing key")
	}
}

func TestEd25519PublicKeyFromCert_RejectsNonEd25519(t *testing.T) {
	// Mint an ECDSA cert instead; mesh must refuse it.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa gen: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(4),
		Subject:      pkix.Name{CommonName: "ecdsa"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	cert, _ := x509.ParseCertificate(der)

	if _, err := ed25519PublicKeyFromCert(cert); err == nil {
		t.Fatal("expected rejection of non-Ed25519 cert")
	}
}
