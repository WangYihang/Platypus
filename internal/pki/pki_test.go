package pki_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
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
		return // satisfy staticcheck
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
	if leaf.Subject.CommonName != "agent-1" {
		t.Fatalf("leaf CN = %q", leaf.Subject.CommonName)
	}
	// NodeID-binding SAN URIs: mesh & AgentLink key off these.
	wantURIs := map[string]bool{
		"platypus://agent/agent-1": false,
		"platypus://project/":      false, // project id is random; prefix check only
	}
	for _, u := range leaf.URIs {
		s := u.String()
		if s == "platypus://agent/agent-1" {
			wantURIs[s] = true
		} else if len(s) > len("platypus://project/") && s[:len("platypus://project/")] == "platypus://project/" {
			wantURIs["platypus://project/"] = true
		}
	}
	for want, ok := range wantURIs {
		if !ok {
			t.Fatalf("leaf missing expected URI SAN prefix %q; got %v", want, leaf.URIs)
		}
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

// IssueAgentLeafFromCSR accepts a PEM PKCS#10 CSR, extracts its
// Ed25519 public key, and issues a leaf cert bound to the server-
// supplied AgentID and ProjectID (CSR subject is ignored).
func TestIssueAgentLeafFromCSR_Roundtrip(t *testing.T) {
	svc, _, admin, proj := setup(t)
	ctx := context.Background()
	if _, err := svc.EnsureCA(ctx, proj.ID, admin.ID); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.CertificateRequest{}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, tmpl, priv)
	if err != nil {
		t.Fatalf("CreateCertificateRequest: %v", err)
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	res, err := svc.IssueAgentLeafFromCSR(ctx, pki.CSRInput{
		ProjectID:    proj.ID,
		AgentID:      "agent-csr",
		CSRPEM:       csrPEM,
		Reason:       "enroll",
		IssuedByUser: admin.ID,
	})
	if err != nil {
		t.Fatalf("IssueAgentLeafFromCSR: %v", err)
	}
	leafBlock, _ := pem.Decode([]byte(res.CertPEM))
	leaf, err := x509.ParseCertificate(leafBlock.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if !leaf.PublicKey.(ed25519.PublicKey).Equal(pub) {
		t.Fatalf("leaf pubkey does not match CSR pubkey")
	}
	if leaf.Subject.CommonName != "agent-csr" {
		t.Fatalf("leaf CN = %q; want agent-csr", leaf.Subject.CommonName)
	}
}

// A CSR carrying a non-Ed25519 key is rejected at parse time; we only
// mint Ed25519 certs today.
func TestIssueAgentLeafFromCSR_RejectsNonEd25519(t *testing.T) {
	svc, _, admin, proj := setup(t)
	ctx := context.Background()
	if _, err := svc.EnsureCA(ctx, proj.ID, admin.ID); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	// Use a malformed PEM to trip parseCSRAndExtractPubkey's decode check.
	_, err := svc.IssueAgentLeafFromCSR(ctx, pki.CSRInput{
		ProjectID: proj.ID, AgentID: "a",
		CSRPEM: []byte("not a csr"),
		Reason: "enroll", IssuedByUser: admin.ID,
	})
	if err == nil {
		t.Fatalf("want error for malformed CSR; got nil")
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

// With the KEK file fallback enabled, an unset env var no longer
// aborts CA init: the KEK is generated on demand, persisted, and
// reused across subsequent calls. This is the zero-config dev path
// exercised by `docker compose up`.
func TestEnsureCA_KEKFileFallback(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	t.Setenv(pki.KEKEnvVar, "")
	kekPath := filepath.Join(t.TempDir(), "nested", "ca.kek")
	prev := pki.KEKPath
	pki.KEKPath = kekPath
	t.Cleanup(func() { pki.KEKPath = prev })

	ctx := context.Background()
	admin := &user.User{ID: "u", Username: "u", PasswordHash: "h", Role: user.RoleAdmin, CreatedAt: time.Now().UTC()}
	if err := db.Users().Create(ctx, admin); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	proj := &storage.Project{ID: "p", Name: "p", Slug: "p", CreatedAt: time.Now().UTC(), CreatedBy: admin.ID}
	if err := db.Projects().Create(ctx, proj); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}

	svc := pki.New(db)
	ca1, err := svc.EnsureCA(ctx, proj.ID, admin.ID)
	if err != nil {
		t.Fatalf("EnsureCA (first): %v", err)
	}

	raw, err := os.ReadFile(kekPath)
	if err != nil {
		t.Fatalf("KEK file not written: %v", err)
	}
	kek, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("KEK file contents not hex: %v", err)
	}
	if len(kek) != 32 {
		t.Fatalf("KEK length = %d; want 32", len(kek))
	}
	info, err := os.Stat(kekPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("KEK file perms = %o; want 0600", perm)
	}

	// Second call must reuse the on-disk KEK (same row returned, no
	// error from a mismatched decrypt).
	ca2, err := svc.EnsureCA(ctx, proj.ID, admin.ID)
	if err != nil {
		t.Fatalf("EnsureCA (second): %v", err)
	}
	if ca1.CertPEM != ca2.CertPEM {
		t.Fatalf("CA cert changed between calls; file reuse broken")
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
