package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/optoken"
	"github.com/WangYihang/Platypus/internal/pki"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// caTestSetup wires the CA handler + registers its routes. Returns
// everything a test might need — router, db, issuer, and the live
// pki.Service so tests can seed certs via the underlying API instead
// of going through admin HTTP flows.
func caTestSetup(t *testing.T) (*gin.Engine, *storage.DB, *pki.Service) {
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

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBAC(db, verifier)
	svc := pki.New(db)
	h := NewCAHandler(db, svc)

	r := gin.New()
	RegisterV1CARoutes(r, h, rbac)
	return r, db, svc
}

func TestCA_InitAndGet(t *testing.T) {
	r, db, _ := caTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p1", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	// POST initialises the CA and returns a parseable self-signed cert.
	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/ca", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /ca: status=%d body=%s", w.Code, w.Body.String())
	}
	var resp caResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	block, _ := pem.Decode([]byte(resp.CertPEM))
	if block == nil {
		t.Fatal("CertPEM decoded empty")
		return // satisfy staticcheck
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || !cert.IsCA {
		t.Fatalf("not a CA cert: err=%v isCA=%v", err, cert != nil && cert.IsCA)
	}

	// GET + second POST both idempotent — same CertPEM.
	for _, method := range []string{"GET", "POST"} {
		w := probeReqWithPath(r, method, "/api/v1/projects/"+proj.ID+"/ca", tok, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s /ca: status=%d", method, w.Code)
		}
		var got caResponse
		_ = json.NewDecoder(w.Body).Decode(&got)
		if got.CertPEM != resp.CertPEM {
			t.Fatalf("%s /ca returned a different CA (non-idempotent)", method)
		}
	}
}

// GET /ca on a project without one returns 404 so admin UI can
// distinguish "initialise me" from an error state.
func TestCA_Get_NotInitialised(t *testing.T) {
	r, db, _ := caTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p1", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	w := probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/ca", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d; want 404", w.Code)
	}
}

// When PKI isn't configured (no KEK), init returns 503 instead of
// silently succeeding on a plaintext key.
func TestCA_Init_KEKMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _ := storage.Open(":memory:")
	t.Cleanup(func() { db.Close() })
	t.Setenv(pki.KEKEnvVar, "")

	cache := optoken.NewCache(64, 30*time.Second)
	verifier := NewTokenVerifier(db, cache)
	rbac := NewRBAC(db, verifier)
	svc := pki.New(db)
	h := NewCAHandler(db, svc)
	r := gin.New()
	RegisterV1CARoutes(r, h, rbac)

	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p1", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/ca", tok, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d; want 503", w.Code)
	}
}

// Full admin flow: init CA → issue two certs → list → revoke one →
// CRL reflects the revocation in both DER and PEM forms.
func TestCA_ListRevokeCRL(t *testing.T) {
	r, db, svc := caTestSetup(t)
	admin := seedUserForAPITest(t, db, "admin", user.RoleAdmin)
	proj := seedProjectForAPITest(t, db, "p1", admin)
	tok := mintBearerForUserID(t, db, admin.ID, user.RoleAdmin)

	w := probeReqWithPath(r, "POST", "/api/v1/projects/"+proj.ID+"/ca", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("init: %d", w.Code)
	}

	pub1, _, _ := ed25519.GenerateKey(rand.Reader)
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)
	ctx := testCtx()
	res1, err := svc.IssueAgentCert(ctx, pki.IssueInput{
		ProjectID: proj.ID, AgentID: "a1", AgentPubKey: pub1,
		Reason: "enroll", IssuedByUser: admin.ID,
	})
	if err != nil {
		t.Fatalf("issue 1: %v", err)
	}
	if _, err := svc.IssueAgentCert(ctx, pki.IssueInput{
		ProjectID: proj.ID, AgentID: "a2", AgentPubKey: pub2,
		Reason: "enroll", IssuedByUser: admin.ID,
	}); err != nil {
		t.Fatalf("issue 2: %v", err)
	}

	w = probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/certs", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
	var lr struct {
		Certs []issuedCertItem `json:"certs"`
	}
	_ = json.NewDecoder(w.Body).Decode(&lr)
	if len(lr.Certs) != 2 {
		t.Fatalf("certs = %d; want 2", len(lr.Certs))
	}

	// Revoke cert #1.
	w = probeReqWithPath(r, "DELETE",
		"/api/v1/projects/"+proj.ID+"/certs/"+strconv.FormatInt(res1.Serial, 10)+"?reason=leaked",
		tok, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d body=%s", w.Code, w.Body.String())
	}

	// CRL in DER form.
	w = probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/crl", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("crl: %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pkix-crl" {
		t.Fatalf("Content-Type = %q", ct)
	}
	crl, err := x509.ParseRevocationList(w.Body.Bytes())
	if err != nil {
		t.Fatalf("parse CRL: %v", err)
	}
	if len(crl.RevokedCertificateEntries) != 1 {
		t.Fatalf("revoked = %d; want 1", len(crl.RevokedCertificateEntries))
	}
	if crl.RevokedCertificateEntries[0].SerialNumber.Int64() != res1.Serial {
		t.Fatalf("wrong serial revoked")
	}

	// CRL in PEM form.
	w = probeReqWithPath(r, "GET", "/api/v1/projects/"+proj.ID+"/crl?format=pem", tok, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("crl pem: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "BEGIN X509 CRL") {
		t.Fatalf("pem body missing header: %s", w.Body.String())
	}

	// Duplicate revoke is 404 (already revoked → no affected rows).
	w = probeReqWithPath(r, "DELETE",
		"/api/v1/projects/"+proj.ID+"/certs/999", tok, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing serial: %d; want 404", w.Code)
	}
}
