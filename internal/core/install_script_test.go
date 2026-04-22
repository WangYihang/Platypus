package core_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/WangYihang/Platypus/internal/app"
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/enrollment"
	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
	"github.com/WangYihang/Platypus/internal/utils/config"
)

// End-to-end: mint an install artifact, curl /install/:token via a
// real *gin.Engine, verify the returned shell script embeds the agent
// bootstrap command with a valid PAT, and verify that a replay curl
// gets a 410 Gone.
func TestInstallScript_HappyAndReplay(t *testing.T) {
	db := mustOpenDB(t)
	ctx := context.Background()
	admin := seedUser(t, db, "admin", user.RoleAdmin)
	proj := seedProject(t, db, "p1", admin)

	svc := enrollment.New(db)
	core.SetEnrollment(svc)
	installCtx(t, db)
	defer uninstallCtx()

	artifact, err := svc.MintInstallArtifact(ctx, enrollment.MintInstallArtifactInput{
		ProjectID: proj.ID, IssuedByUser: admin.ID,
		ServerEndpoint: "127.0.0.1:13337",
	})
	if err != nil {
		t.Fatalf("MintInstallArtifact: %v", err)
	}

	// Stand up just the distributor engine. We don't need the TLS
	// listener path — the install endpoint is pure HTTP.
	gin.SetMode(gin.TestMode)
	engine := core.CreateDistributorServer("127.0.0.1", 0, "")

	// First curl: 200 OK, body is a shell script containing the PAT.
	req := httptest.NewRequest("GET", "/install/"+artifact.PlaintextDownloadToken, nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("first curl status = %d; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "#!/bin/sh") {
		t.Fatalf("body missing shebang: %s", body)
	}
	if !strings.Contains(body, "AGENT_TOKEN='plt_") {
		t.Fatalf("body missing AGENT_TOKEN line: %s", body)
	}
	if !strings.Contains(body, "AGENT_HOST='127.0.0.1'") {
		t.Fatalf("body missing AGENT_HOST: %s", body)
	}
	if !strings.Contains(body, "AGENT_PORT='13337'") {
		t.Fatalf("body missing AGENT_PORT: %s", body)
	}

	// Verify the install_download_tokens row moved to consumed state.
	tok, err := db.InstallDownloadTokens().Get(ctx, artifact.DownloadID)
	if err != nil {
		t.Fatalf("Get install token: %v", err)
	}
	if tok.ConsumedAt == nil {
		t.Fatal("install token not marked consumed")
	}
	if tok.ConsumedPATID == "" {
		t.Fatal("install token missing consumed_pat_id")
	}

	// Replay: 410 Gone.
	req2 := httptest.NewRequest("GET", "/install/"+artifact.PlaintextDownloadToken, nil)
	rec2 := httptest.NewRecorder()
	engine.ServeHTTP(rec2, req2)
	if rec2.Code != 410 {
		t.Fatalf("replay status = %d; want 410", rec2.Code)
	}
}

// Unknown token → 404.
func TestInstallScript_Unknown(t *testing.T) {
	db := mustOpenDB(t)
	core.SetEnrollment(enrollment.New(db))
	installCtx(t, db)
	defer uninstallCtx()

	gin.SetMode(gin.TestMode)
	engine := core.CreateDistributorServer("127.0.0.1", 0, "")

	req := httptest.NewRequest("GET", "/install/dl_aaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbbbbbb", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("status = %d; want 404", rec.Code)
	}
}

// When the enrollment service is unconfigured, the endpoint must
// refuse cleanly rather than 500.
func TestInstallScript_NoEnrollmentService(t *testing.T) {
	// Reset the enrollment global to simulate a server started without it.
	core.SetEnrollment(nil)

	gin.SetMode(gin.TestMode)
	// We still need Ctx.Distributor to be non-nil for CreateDistributorServer
	// to succeed; install dummy context.
	db := mustOpenDB(t)
	installCtx(t, db)
	defer uninstallCtx()

	engine := core.CreateDistributorServer("127.0.0.1", 0, "")
	req := httptest.NewRequest("GET", "/install/dl_whatever.secret", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Fatalf("status = %d; want 503", rec.Code)
	}
}

// Silence unused-import warnings when tests are shrunk.
var _ = app.App{}
var _ = (*storage.DB)(nil)
var _ = (*config.Config)(nil)
