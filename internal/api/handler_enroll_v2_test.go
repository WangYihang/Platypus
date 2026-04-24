package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/pki"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// enrollV2TestSetup wires the enroll-v2 handler with an in-memory DB,
// fresh KEK, pre-minted project + PAT. Returns the router, the raw
// PAT plaintext the agent should present, and the project's CA.
func enrollV2TestSetup(t *testing.T) (*gin.Engine, string, *storage.Project) {
	t.Helper()
	gin.SetMode(gin.TestMode)

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
		ID: "user-admin", Username: "admin", PasswordHash: "x",
		Role: user.RoleAdmin, CreatedAt: time.Now().UTC(),
	}
	if err := db.Users().Create(ctx, admin); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	proj := &storage.Project{
		ID: "proj-1", Name: "P1", Slug: "p1",
		CreatedAt: time.Now().UTC(), CreatedBy: admin.ID,
	}
	if err := db.Projects().Create(ctx, proj); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}

	pkiSvc := pki.New(db)
	if _, err := pkiSvc.EnsureCA(ctx, proj.ID, admin.ID); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	enrollSvc := enrollment.New(db).WithPKI(pkiSvc)

	// Mint a single-use PAT bound to this project.
	patRes, err := enrollSvc.MintPAT(ctx, enrollment.MintPATInput{
		ProjectID:    proj.ID,
		MaxUses:      1,
		TTL:          time.Hour,
		IssuedByUser: admin.ID,
	})
	if err != nil {
		t.Fatalf("MintPAT: %v", err)
	}

	h := NewEnrollV2Handler(enrollSvc, pkiSvc)
	r := gin.New()
	RegisterV2AgentEnrollRoute(r, h)
	return r, patRes.PlaintextToken, proj
}

// generateEd25519CSR returns a PEM PKCS#10 CSR signed with a fresh
// Ed25519 key. Subject is left empty — the server ignores it.
func generateEd25519CSR(t *testing.T) (csrPEM []byte, pub ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, priv)
	if err != nil {
		t.Fatalf("CreateCertificateRequest: %v", err)
	}
	csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
	return
}

// Happy path: POST EnrollRequest with valid PAT + fresh CSR → 200
// with a leaf cert signed by the project CA and bound to a new
// agent_id.
func TestEnrollV2_HappyPath(t *testing.T) {
	r, pat, proj := enrollV2TestSetup(t)
	csrPEM, csrPub := generateEd25519CSR(t)

	reqBody, err := proto.Marshal(&v2pb.EnrollRequest{
		Pat:      pat,
		CsrPem:   csrPEM,
		Hostname: "unit-test-host",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/enroll",
		bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", ContentTypeEnrollV2)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	var resp v2pb.EnrollResponse
	if err := proto.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ProjectId != proj.ID {
		t.Fatalf("project_id = %q; want %q", resp.ProjectId, proj.ID)
	}
	if resp.AgentId == "" {
		t.Fatal("agent_id empty")
	}
	if len(resp.CertPem) == 0 || len(resp.CaPem) == 0 {
		t.Fatal("cert or ca PEM empty")
	}
	if resp.CertExpiresUnix <= time.Now().Unix() {
		t.Fatalf("cert expiry %d is not in the future", resp.CertExpiresUnix)
	}

	// Cert must carry the pubkey from the CSR (not some other key).
	leafBlock, _ := pem.Decode(resp.CertPem)
	leaf, err := x509.ParseCertificate(leafBlock.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if !leaf.PublicKey.(ed25519.PublicKey).Equal(csrPub) {
		t.Fatal("leaf pubkey does not match CSR pubkey")
	}
}

// Malformed request body → 400 without touching the PAT.
func TestEnrollV2_MalformedBody(t *testing.T) {
	r, _, _ := enrollV2TestSetup(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/enroll",
		bytes.NewReader([]byte("not a protobuf")))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400; body = %s", w.Code, w.Body.String())
	}
}

// Bogus PAT → 401.
func TestEnrollV2_InvalidPAT(t *testing.T) {
	r, _, _ := enrollV2TestSetup(t)
	csrPEM, _ := generateEd25519CSR(t)
	body, _ := proto.Marshal(&v2pb.EnrollRequest{
		Pat:    "plt_bogus.deadbeef",
		CsrPem: csrPEM,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/enroll",
		bytes.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want 401; body = %s", w.Code, w.Body.String())
	}
}

// Missing CSR → 400.
func TestEnrollV2_MissingCSR(t *testing.T) {
	r, pat, _ := enrollV2TestSetup(t)
	body, _ := proto.Marshal(&v2pb.EnrollRequest{Pat: pat})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/enroll",
		bytes.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400; body = %s", w.Code, w.Body.String())
	}
}
