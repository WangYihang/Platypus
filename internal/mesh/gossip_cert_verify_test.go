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

	"google.golang.org/protobuf/proto"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// mintCAAndLeaf mints a fresh self-signed CA + a leaf cert chained
// to it, with the given agentID baked into the leaf's URI SAN.
// Returns the CA cert, the leaf PEM, and the leaf's private key.
func mintCAAndLeaf(t *testing.T, agentID, projectID string) (caCert *x509.Certificate, leafPEM []byte, leafPriv ed25519.PrivateKey) {
	t.Helper()

	// Self-signed CA.
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:         true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caPub, caPriv)
	if err != nil {
		t.Fatalf("create ca: %v", err)
	}
	caCert, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}

	// Leaf signed by the CA.
	leafPub, leafPrivKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	agentURI, _ := url.Parse("platypus://agent/" + agentID)
	projectURI, _ := url.Parse("platypus://project/" + projectID)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: agentID},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{agentURI, projectURI},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, leafPub, caPriv)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	leafPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafPriv = leafPrivKey
	return
}

// TestPeerDelta_CertBoundVerify_HappyPath mints a CA, issues one
// leaf under it, builds a signed PeerDelta with OriginCertPem
// populated, and verifies it against a pool containing only that
// CA. Pass.
func TestPeerDelta_CertBoundVerify_HappyPath(t *testing.T) {
	ca, leafPEM, leafPriv := mintCAAndLeaf(t, "agent-dx", "proj-v")
	pool := x509.NewCertPool()
	pool.AddCert(ca)

	pub := leafPriv.Public().(ed25519.PublicKey)
	delta := &v2pb.MeshPeerDelta{
		OriginNodeId:  "agent-dx",
		Pubkey:        pub,
		Seq:           1,
		Ttl:           5,
		OriginCertPem: leafPEM,
	}
	signPeerDelta(leafPriv, delta)
	if err := verifyPeerDelta(delta, pool); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// TestPeerDelta_CertBoundVerify_RejectsUntrustedCA proves a leaf
// signed by a *different* CA fails chain verification, even with a
// correct signature and matching SAN.
func TestPeerDelta_CertBoundVerify_RejectsUntrustedCA(t *testing.T) {
	_, leafPEM, leafPriv := mintCAAndLeaf(t, "agent-dx", "proj-v")
	// Trust pool contains a DIFFERENT CA.
	otherCA, _, _ := mintCAAndLeaf(t, "decoy", "proj-v")
	pool := x509.NewCertPool()
	pool.AddCert(otherCA)

	pub := leafPriv.Public().(ed25519.PublicKey)
	delta := &v2pb.MeshPeerDelta{
		OriginNodeId:  "agent-dx",
		Pubkey:        pub,
		Seq:           1,
		Ttl:           5,
		OriginCertPem: leafPEM,
	}
	signPeerDelta(leafPriv, delta)
	if err := verifyPeerDelta(delta, pool); err == nil {
		t.Fatal("expected chain verification to fail against untrusted CA pool")
	}
}

// TestPeerDelta_CertBoundVerify_RejectsSANMismatch: cert says
// "agent-a" but the delta claims origin is "agent-b". Chain passes
// but identity check catches the lie.
func TestPeerDelta_CertBoundVerify_RejectsSANMismatch(t *testing.T) {
	ca, leafPEM, leafPriv := mintCAAndLeaf(t, "agent-a", "proj-v")
	pool := x509.NewCertPool()
	pool.AddCert(ca)

	pub := leafPriv.Public().(ed25519.PublicKey)
	delta := &v2pb.MeshPeerDelta{
		OriginNodeId:  "agent-b", // lie
		Pubkey:        pub,
		Seq:           1,
		Ttl:           5,
		OriginCertPem: leafPEM,
	}
	signPeerDelta(leafPriv, delta)
	if err := verifyPeerDelta(delta, pool); err == nil {
		t.Fatal("expected SAN mismatch to fail")
	}
}

// TestPeerDelta_LegacyPathStillWorksWithoutCert verifies the
// fallback: no OriginCertPem + no trustedCAs → legacy DeriveNodeID
// check is used.
func TestPeerDelta_LegacyPathStillWorksWithoutCert(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	delta := &v2pb.MeshPeerDelta{
		OriginNodeId: DeriveNodeID(pub),
		Pubkey:       pub,
		Seq:          1,
		Ttl:          5,
	}
	signPeerDelta(priv, delta)
	if err := verifyPeerDelta(delta, nil); err != nil {
		t.Fatalf("legacy verify: %v", err)
	}
}

// TestLSDB_CertBoundIngestWithPool exercises the LSA path, which
// is the one the router depends on. Same patterns as PeerDelta.
func TestLSDB_CertBoundIngestWithPool(t *testing.T) {
	ca, leafPEM, leafPriv := mintCAAndLeaf(t, "agent-lsa", "proj-v")
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	pub := leafPriv.Public().(ed25519.PublicKey)

	lsa := &v2pb.MeshLSA{
		OriginNodeId:  "agent-lsa",
		Seq:           7,
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
		Pubkey:        pub,
		FloodTtl:      maxFloodTTL,
		OriginCertPem: leafPEM,
		Links:         []*v2pb.MeshLSA_Link{{NodeId: "other", Cost: 1}},
	}
	// Sign with Sig blanked and flood_ttl blanked (LSDB.Ingest uses
	// the same canonical form).
	canonCopy := proto.Clone(lsa).(*v2pb.MeshLSA)
	canonCopy.Sig = nil
	canonCopy.FloodTtl = 0
	canon, err := canonicalBytesForSig(canonCopy)
	if err != nil {
		t.Fatalf("canonical marshal: %v", err)
	}
	lsa.Sig = signBytes(leafPriv, canon)

	db := newLSDB()
	changed, err := db.Ingest(lsa, pool)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if !changed {
		t.Fatal("Ingest changed=false on fresh LSA")
	}
}
