package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Package-shared test CA: one self-signed CA for the whole mesh
// test binary. Each call to mustIdentity / mintLeafForTest mints a
// fresh leaf signed by this CA and loads it through
// LoadIdentityFromCert, so mesh tests exercise the same cert-bound
// identity path that a production agent uses after enrollment.

var (
	testCAOnce   sync.Once
	testCACert   *x509.Certificate
	testCAPriv   ed25519.PrivateKey
	testCAPool   *x509.CertPool
	testCASerial atomic.Int64
)

// ensureTestCA lazily mints the shared self-signed CA. Safe to
// call from any parallel test goroutine — sync.Once handles the
// race.
func ensureTestCA(t *testing.T) {
	t.Helper()
	testCAOnce.Do(func() {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("mesh test CA keygen: %v", err)
		}
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "mesh-test-ca"},
			NotBefore:             time.Now().Add(-time.Hour),
			NotAfter:              time.Now().Add(24 * time.Hour),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
			IsCA:                  true,
			BasicConstraintsValid: true,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
		if err != nil {
			t.Fatalf("mesh test CA create: %v", err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatalf("mesh test CA parse: %v", err)
		}
		testCACert = cert
		testCAPriv = priv
		testCAPool = x509.NewCertPool()
		testCAPool.AddCert(cert)
	})
}

// nextSerial bumps a process-wide counter so every leaf in the
// test binary gets a unique certificate serial.
func nextSerial() int64 { return testCASerial.Add(1) + 100 }

// randAgentID mints an unpredictable agent id so separate
// mustIdentity calls don't collide on NodeID in tests that
// compare identities. 8 hex chars of crypto random.
func randAgentID(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return "agent-" + hex.EncodeToString(b[:])
}

// mustIdentity mints a unique cert-bound Identity for tests.
// Wraps mintLeafFromTestCA + LoadIdentityFromCert.
func mustIdentity(t *testing.T) *Identity {
	t.Helper()
	certPEM, keyPEM := mintLeafFromTestCA(t, randAgentID(t))
	id, err := LoadIdentityFromCert(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadIdentityFromCert: %v", err)
	}
	return id
}

// mintLeafFromTestCA mints a fresh leaf signed by the shared test
// CA and returns (certPEM, keyPEM) in the formats
// LoadIdentityFromCert expects. The SAN carries
// platypus://agent/<agentID>.
func mintLeafFromTestCA(t *testing.T, agentID string) (certPEM, keyPEM []byte) {
	t.Helper()
	ensureTestCA(t)
	leafPub, leafPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("leaf keygen: %v", err)
	}
	agentURI, _ := url.Parse("platypus://agent/" + agentID)
	projectURI, _ := url.Parse("platypus://project/mesh-test")
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(nextSerial()),
		Subject:      pkix.Name{CommonName: agentID},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{agentURI, projectURI},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, tmpl, testCACert, leafPub, testCAPriv)
	if err != nil {
		t.Fatalf("leaf create: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafPriv)
	if err != nil {
		t.Fatalf("leaf key marshal: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return
}
