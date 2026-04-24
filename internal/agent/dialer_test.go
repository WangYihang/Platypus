package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// BuildDialerTLSConfig wires an Identity into a *tls.Config the
// agent uses for tls.Dial to the server: own leaf cert as
// Certificates, project CA as RootCAs, ALPN "http/1.1" (the
// HTTP-layer ALPN that lands on the server's unified ingress HTTP
// listener where /api/v1/agent/link lives).

// mintTestIdentity mints a self-signed CA and a leaf cert signed
// by it, returning everything packaged as an Identity.
func mintTestIdentity(t *testing.T) *Identity {
	t.Helper()
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ca keygen: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "dialer-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caPub, caPriv)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	leafPub, leafPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("leaf keygen: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "agent-test"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, leafPub, caPriv)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	return &Identity{PrivateKey: leafPriv, CertPEM: leafPEM, CAPEM: caPEM}
}

func TestBuildDialerTLSConfig_HappyPath(t *testing.T) {
	id := mintTestIdentity(t)
	cfg, err := BuildDialerTLSConfig(id)
	if err != nil {
		t.Fatalf("BuildDialerTLSConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg nil")
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates len = %d; want 1", len(cfg.Certificates))
	}
	if cfg.Certificates[0].PrivateKey == nil {
		t.Fatal("Certificates[0] missing private key")
	}
	if cfg.RootCAs == nil {
		t.Fatal("RootCAs nil; should contain project CA")
	}
	if len(cfg.NextProtos) == 0 || cfg.NextProtos[0] != "http/1.1" {
		t.Fatalf("NextProtos = %v; want [http/1.1 ...]", cfg.NextProtos)
	}
	if cfg.MinVersion < tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x; want >= TLS 1.2", cfg.MinVersion)
	}
	if cfg.InsecureSkipVerify {
		t.Fatal("InsecureSkipVerify must be false in v2")
	}
}

func TestBuildDialerTLSConfig_NilIdentity(t *testing.T) {
	if _, err := BuildDialerTLSConfig(nil); err == nil {
		t.Fatal("want error on nil identity")
	}
}

func TestBuildDialerTLSConfig_RejectsMalformedCert(t *testing.T) {
	id := &Identity{
		PrivateKey: make(ed25519.PrivateKey, ed25519.PrivateKeySize),
		CertPEM:    []byte("-----BEGIN CERTIFICATE-----\nGARBAGE\n-----END CERTIFICATE-----\n"),
		CAPEM:      []byte("-----BEGIN CERTIFICATE-----\nMORE GARBAGE\n-----END CERTIFICATE-----\n"),
	}
	if _, err := BuildDialerTLSConfig(id); err == nil {
		t.Fatal("want error on malformed cert PEM")
	}
}

func TestBuildDialerTLSConfig_RejectsMalformedCA(t *testing.T) {
	id := mintTestIdentity(t)
	id.CAPEM = []byte("not a PEM")
	if _, err := BuildDialerTLSConfig(id); err == nil {
		t.Fatal("want error on malformed CA PEM")
	}
}
