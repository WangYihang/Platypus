package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

// LoadProjectCA owns a single piece of wire-format parsing: take the
// PLATYPUS_PROJECT_CA environment variable (base64-encoded PEM, as
// injected by the install script) and yield an *x509.CertPool the
// rest of the agent can use as RootCAs. These tests pin the exact
// error contract so the dialer can branch cleanly on each failure.

// Empty env var is a first-class case: pre-v2 install scripts don't
// set it, and we want the caller to fall back to "skip verify" (or
// to refuse to connect, depending on operator policy) rather than
// see a vague parse error.
func TestLoadProjectCA_EmptyEnvReturnsNil(t *testing.T) {
	pool, err := LoadProjectCA("")
	if err != nil {
		t.Fatalf("LoadProjectCA(\"\") err = %v; want nil", err)
	}
	if pool != nil {
		t.Fatalf("LoadProjectCA(\"\") pool = %v; want nil", pool)
	}
}

// A garbled base64 value surfaces a specific sentinel so the dialer
// can log-and-bail rather than continuing with a blank pool.
func TestLoadProjectCA_InvalidBase64(t *testing.T) {
	if _, err := LoadProjectCA("not valid base64 !!!"); !errors.Is(err, ErrProjectCABadBase64) {
		t.Fatalf("err = %v; want %v", err, ErrProjectCABadBase64)
	}
}

// Base64 that decodes but doesn't contain a PEM block is rejected.
func TestLoadProjectCA_InvalidPEM(t *testing.T) {
	input := base64.StdEncoding.EncodeToString([]byte("this is not a PEM certificate"))
	if _, err := LoadProjectCA(input); !errors.Is(err, ErrProjectCABadPEM) {
		t.Fatalf("err = %v; want %v", err, ErrProjectCABadPEM)
	}
}

// PEM block with CERTIFICATE header but bogus DER is rejected.
func TestLoadProjectCA_InvalidDER(t *testing.T) {
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("garbage der")})
	input := base64.StdEncoding.EncodeToString(block)
	if _, err := LoadProjectCA(input); !errors.Is(err, ErrProjectCABadCert) {
		t.Fatalf("err = %v; want %v", err, ErrProjectCABadCert)
	}
}

// Happy path: a self-signed Ed25519 CA parses into a pool with
// exactly that cert, and the cert is recognised as a root by
// x509.Verify when used as the only trust anchor.
func TestLoadProjectCA_HappyPath(t *testing.T) {
	caPEM := mustGenerateSelfSignedCA(t, "trust-test-ca")
	input := base64.StdEncoding.EncodeToString(caPEM)

	pool, err := LoadProjectCA(input)
	if err != nil {
		t.Fatalf("LoadProjectCA: %v", err)
	}
	if pool == nil {
		t.Fatal("LoadProjectCA returned nil pool on happy path")
	}
	// Round-trip check: decode the CA, verify it against the pool
	// we just built. Self-signed + pool-of-one → chain length 1.
	block, _ := pem.Decode(caPEM)
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}
	chains, err := ca.Verify(x509.VerifyOptions{Roots: pool})
	if err != nil {
		t.Fatalf("CA failed to verify against pool built from itself: %v", err)
	}
	if len(chains) != 1 || len(chains[0]) != 1 {
		t.Fatalf("expected a single-cert chain; got %v", chains)
	}
}

// mustGenerateSelfSignedCA mints a brand-new Ed25519 CA cert with a
// short validity. Only used to stuff PEM into LoadProjectCA — not a
// production-shaped CA.
func mustGenerateSelfSignedCA(t *testing.T, cn string) []byte {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
