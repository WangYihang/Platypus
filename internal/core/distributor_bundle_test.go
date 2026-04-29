package core

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	"github.com/WangYihang/Platypus/pkg/installbundle"
)

// TestServeInstallScript_BundleFormat exercises the
// `?format=bundle` branch end-to-end: mint an install token, GET the
// bundle URL, decode the response, verify the round-trip carried
// ServerEndpoint / PAT / CA back faithfully. Same code path the
// agent CLI walks when the user pasted a `pinst_` token.
func TestServeInstallScript_BundleFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Minimal user / project seed so the install-token mint can take
	// ownership of a real row.
	admin := &user.User{
		ID: "u-admin", Username: "admin", PasswordHash: "h",
		Role: user.RoleAdmin, CreatedAt: time.Now().UTC(),
	}
	if err := db.Users().Create(t.Context(), admin); err != nil {
		t.Fatalf("Users.Create: %v", err)
	}
	if err := db.Projects().Create(t.Context(), &storage.Project{
		ID: "p1", Slug: "p1", Name: "Bundle test",
		CreatedAt: time.Now().UTC(), CreatedBy: admin.ID,
	}); err != nil {
		t.Fatalf("Projects.Create: %v", err)
	}

	svc := enrollment.New(db)
	SetEnrollment(svc)
	t.Cleanup(func() { SetEnrollment(nil) })

	mint, err := svc.MintInstallArtifact(t.Context(), enrollment.MintInstallArtifactInput{
		ProjectID:      "p1",
		IssuedByUser:   admin.ID,
		ServerEndpoint: "agent.example.com:13337",
		TTL:            time.Minute,
		PATTTL:         time.Minute,
	})
	if err != nil {
		t.Fatalf("MintInstallArtifact: %v", err)
	}

	// Install token consumed once below; bundle response is the only
	// chance to retrieve the PAT.
	r := gin.New()
	r.GET("/api/v1/install/:token", serveInstallScript)

	req := httptest.NewRequest("GET",
		"/api/v1/install/"+mint.PlaintextDownloadToken+"?format=bundle", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type=%q, want text/plain", ct)
	}

	wire := strings.TrimSpace(w.Body.String())
	if !installbundle.Looks(wire) {
		t.Fatalf("body doesn't look like a pinst_ bundle: %q", wire)
	}
	got, err := installbundle.Decode(wire)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Server != "agent.example.com:13337" {
		t.Errorf("Server=%q, want agent.example.com:13337", got.Server)
	}
	if !strings.HasPrefix(got.PAT, "plt_") {
		t.Errorf("PAT=%q, want plt_*", got.PAT)
	}
	if got.ProjectID != "p1" {
		t.Errorf("ProjectID=%q, want p1", got.ProjectID)
	}
}

// TestServeInstallScript_BundleConsumeAlreadyUsed: the bundle path
// consumes the install token, so a second curl gets 410 Gone — both
// the script flow and the bundle flow share the same single-use
// invariant.
func TestServeInstallScript_BundleConsumeAlreadyUsed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	admin := &user.User{
		ID: "u", Username: "u", PasswordHash: "h",
		Role: user.RoleAdmin, CreatedAt: time.Now().UTC(),
	}
	_ = db.Users().Create(t.Context(), admin)
	_ = db.Projects().Create(t.Context(), &storage.Project{
		ID: "p1", Slug: "p1", Name: "test",
		CreatedAt: time.Now().UTC(), CreatedBy: admin.ID,
	})
	svc := enrollment.New(db)
	SetEnrollment(svc)
	t.Cleanup(func() { SetEnrollment(nil) })

	mint, _ := svc.MintInstallArtifact(t.Context(), enrollment.MintInstallArtifactInput{
		ProjectID: "p1", IssuedByUser: admin.ID,
		ServerEndpoint: "h:1", TTL: time.Minute, PATTTL: time.Minute,
	})

	r := gin.New()
	r.GET("/api/v1/install/:token", serveInstallScript)

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest("GET",
		"/api/v1/install/"+mint.PlaintextDownloadToken+"?format=bundle", nil))
	if first.Code != 200 {
		t.Fatalf("first GET status=%d", first.Code)
	}

	second := httptest.NewRecorder()
	r.ServeHTTP(second, httptest.NewRequest("GET",
		"/api/v1/install/"+mint.PlaintextDownloadToken+"?format=bundle", nil))
	if second.Code != 410 {
		t.Fatalf("second GET status=%d body=%s, want 410 Gone", second.Code, second.Body.String())
	}
}
