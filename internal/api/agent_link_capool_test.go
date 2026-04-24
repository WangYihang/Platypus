package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/pki"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// ProjectsCAPool builds an *x509.CertPool from every project CA in
// storage. The function is re-invoked on each server request so new
// projects (or rotated CAs) pick up without restarting the process.

func TestProjectsCAPool_EmptyDB(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	pool := ProjectsCAPool(db)()
	if pool == nil {
		t.Fatal("ProjectsCAPool() returned nil pool; want empty non-nil")
	}
}

// Seeding a project CA yields a pool that trusts it. We use the
// pki.Service directly to mint the CA (rather than assembling it by
// hand) so the test also catches integration regressions.
func TestProjectsCAPool_IncludesSeededProjectCAs(t *testing.T) {
	db, admin, proj := seedProjectWithCA(t)

	pool := ProjectsCAPool(db)()

	// Build a leaf signed by the seeded CA and confirm it verifies
	// against the pool.
	leafPEM := mintLeafSignedBySeededCA(t, db, admin.ID, proj.ID)
	block, _ := pem.Decode([]byte(leafPEM))
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Fatalf("leaf failed to verify against ProjectsCAPool: %v", err)
	}
}

// Standalone helpers.

func seedProjectWithCA(t *testing.T) (*storage.DB, *user.User, *storage.Project) {
	t.Helper()
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	kek := make([]byte, 32)
	_, _ = rand.Read(kek)
	t.Setenv(pki.KEKEnvVar, hex.EncodeToString(kek))

	ctx := context.Background()
	admin := &user.User{
		ID: "user-admin", Username: "admin", PasswordHash: "h",
		Role: user.RoleAdmin, CreatedAt: time.Now().UTC(),
	}
	if err := db.Users().Create(ctx, admin); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	proj := &storage.Project{
		ID: "proj-ca", Name: "CA", Slug: "ca",
		CreatedAt: time.Now().UTC(), CreatedBy: admin.ID,
	}
	if err := db.Projects().Create(ctx, proj); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}
	if _, err := pki.New(db).EnsureCA(ctx, proj.ID, admin.ID); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	return db, admin, proj
}

func mintLeafSignedBySeededCA(t *testing.T, db *storage.DB, adminID, projectID string) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	_ = pkix.Name{} // silence "imported and not used" if deps shift
	_ = big.NewInt(0)
	res, err := pki.New(db).IssueAgentCert(context.Background(), pki.IssueInput{
		ProjectID: projectID, AgentID: "a-leaf", AgentPubKey: pub,
		Reason: "enroll", IssuedByUser: adminID,
	})
	if err != nil {
		t.Fatalf("IssueAgentCert: %v", err)
	}
	return res.CertPEM
}
