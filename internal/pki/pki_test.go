package pki_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/pki"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// setup opens a temp DB, seeds a user + project, configures the KEK env
// var for the test's lifetime, and returns a ready-to-use Service.
func setup(t *testing.T) (*pki.Service, *storage.DB, *user.User, *storage.Project) {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Random KEK per test. t.Setenv unsets automatically on t.Cleanup.
	kek := make([]byte, 32)
	if _, err := rand.Read(kek); err != nil {
		t.Fatalf("rand: %v", err)
	}
	t.Setenv(pki.KEKEnvVar, hex.EncodeToString(kek))

	ctx := context.Background()
	admin := &user.User{
		ID:           "user-admin",
		Username:     "admin",
		PasswordHash: "hash",
		Role:         user.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	}
	if err := db.Users().Create(ctx, admin); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	proj := &storage.Project{
		ID:        "proj-1",
		Name:      "Project 1",
		Slug:      "p1",
		CreatedAt: time.Now().UTC(),
		CreatedBy: admin.ID,
	}
	if err := db.Projects().Create(ctx, proj); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	return pki.New(db), db, admin, proj
}

// EnsureCA is idempotent — calling it twice returns the same CA row,
// not two different ones (the second call just Get's).
func TestEnsureCA_IdempotentAndPersistent(t *testing.T) {
	svc, db, admin, proj := setup(t)
	ctx := context.Background()

	first, err := svc.EnsureCA(ctx, proj.ID, admin.ID)
	if err != nil {
		t.Fatalf("EnsureCA first: %v", err)
	}
	second, err := svc.EnsureCA(ctx, proj.ID, admin.ID)
	if err != nil {
		t.Fatalf("EnsureCA second: %v", err)
	}
	if first.CertPEM != second.CertPEM {
		t.Fatal("EnsureCA minted a second CA; must be idempotent")
	}
	// Cert is a valid self-signed Ed25519 root.
	block, _ := pem.Decode([]byte(first.CertPEM))
	if block == nil {
		t.Fatal("CA CertPEM decoded empty")
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	if !caCert.IsCA {
		t.Fatal("CA cert missing BasicConstraintsValid/IsCA")
	}
	// Private key persisted encrypted.
	row, err := db.ProjectCA().Get(ctx, proj.ID)
	if err != nil {
		t.Fatalf("ProjectCA.Get: %v", err)
	}
	if len(row.PrivKeyNonce) != 12 {
		t.Fatalf("nonce length = %d; want 12", len(row.PrivKeyNonce))
	}
	if len(row.PrivKeyCT) == 0 {
		t.Fatal("privkey ciphertext empty")
	}
}

// IssueAgentCert returns a cert that validates against the CA and
// binds the correct agent pubkey.
func TestIssueAgentCert_ValidChain(t *testing.T) {
	svc, _, admin, proj := setup(t)
	ctx := context.Background()

	if _, err := svc.EnsureCA(ctx, proj.ID, admin.ID); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	res, err := svc.IssueAgentCert(ctx, pki.IssueInput{
		ProjectID:    proj.ID,
		AgentID:      "agent-1",
		AgentPubKey:  pub,
		Reason:       "enroll",
		IssuedByUser: admin.ID,
	})
	if err != nil {
		t.Fatalf("IssueAgentCert: %v", err)
	}
	if res.Serial != 1 {
		t.Fatalf("first serial = %d; want 1", res.Serial)
	}

	// Parse both PEMs. Leaf's issuer must match the CA's subject, and
	// VerifyX509 against a pool containing the CA must succeed.
	leafBlock, _ := pem.Decode([]byte(res.CertPEM))
	leaf, err := x509.ParseCertificate(leafBlock.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if leaf.Subject.CommonName != "Platypus agent agent-1" {
		t.Fatalf("leaf CN = %q", leaf.Subject.CommonName)
	}
	caBlock, _ := pem.Decode([]byte(res.CAPem))
	caCert, _ := x509.ParseCertificate(caBlock.Bytes)

	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:       pool,
		CurrentTime: time.Now(),
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Fatalf("leaf failed to verify against CA: %v", err)
	}
}

// Second issue gets serial=2, lands in issued_certs, both rows visible
// via ListByProject newest-first.
func TestIssueAgentCert_AllocatesIncrementingSerial(t *testing.T) {
	svc, db, admin, proj := setup(t)
	ctx := context.Background()

	if _, err := svc.EnsureCA(ctx, proj.ID, admin.ID); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	for i := 0; i < 3; i++ {
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		if _, err := svc.IssueAgentCert(ctx, pki.IssueInput{
			ProjectID: proj.ID, AgentID: "a", AgentPubKey: pub,
			Reason: "enroll", IssuedByUser: admin.ID,
		}); err != nil {
			t.Fatalf("issue[%d]: %v", i, err)
		}
	}
	rows, err := db.IssuedCerts().ListByProject(ctx, proj.ID, false, time.Now())
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("issued count = %d; want 3", len(rows))
	}
	// Descending serial → newest first
	if rows[0].Serial != 3 || rows[2].Serial != 1 {
		t.Fatalf("serial order = [%d..%d]; want [3..1]", rows[0].Serial, rows[2].Serial)
	}
}

// BuildCRL includes revoked-live certs and excludes expired ones.
func TestBuildCRL_IncludesRevokedLive(t *testing.T) {
	svc, db, admin, proj := setup(t)
	ctx := context.Background()

	if _, err := svc.EnsureCA(ctx, proj.ID, admin.ID); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	issued, err := svc.IssueAgentCert(ctx, pki.IssueInput{
		ProjectID: proj.ID, AgentID: "a", AgentPubKey: pub,
		Reason: "enroll", IssuedByUser: admin.ID,
	})
	if err != nil {
		t.Fatalf("IssueAgentCert: %v", err)
	}

	// Revoke it. CRL should now contain the serial.
	if err := db.IssuedCerts().Revoke(ctx, proj.ID, issued.Serial, admin.ID, "test", time.Now()); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	crlDER, err := svc.BuildCRL(ctx, proj.ID)
	if err != nil {
		t.Fatalf("BuildCRL: %v", err)
	}
	crl, err := x509.ParseRevocationList(crlDER)
	if err != nil {
		t.Fatalf("parse CRL: %v", err)
	}
	if n := len(crl.RevokedCertificateEntries); n != 1 {
		t.Fatalf("CRL entries = %d; want 1", n)
	}
	if crl.RevokedCertificateEntries[0].SerialNumber.Int64() != issued.Serial {
		t.Fatalf("CRL serial mismatch")
	}
}

// Missing KEK fails CA init cleanly rather than panicking.
func TestEnsureCA_MissingKEK(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	t.Setenv(pki.KEKEnvVar, "")

	admin := &user.User{ID: "u", Username: "u", PasswordHash: "h", Role: user.RoleAdmin, CreatedAt: time.Now().UTC()}
	_ = db.Users().Create(context.Background(), admin)
	proj := &storage.Project{ID: "p", Name: "p", Slug: "p", CreatedAt: time.Now().UTC(), CreatedBy: admin.ID}
	_ = db.Projects().Create(context.Background(), proj)

	svc := pki.New(db)
	if _, err := svc.EnsureCA(context.Background(), proj.ID, admin.ID); err != pki.ErrKEKMissing {
		t.Fatalf("err = %v; want ErrKEKMissing", err)
	}
}
