package enrollment_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/WangYihang/Platypus/internal/enrollment"
)

// Happy path: admin mints an install artifact → curl consumes it once
// → fresh PAT is usable once → second attempt is already_consumed.
func TestConsumeInstallDownload_HappyPath(t *testing.T) {
	svc := newSvc(t)
	admin, proj := bootstrap(t, svc.DB())
	ctx := context.Background()

	artifact, err := svc.Svc.MintInstallArtifact(ctx, enrollment.MintInstallArtifactInput{
		ProjectID:      proj.ID,
		IssuedByUser:   admin.ID,
		ServerEndpoint: "127.0.0.1:13337",
		TargetOS:       "linux",
		TargetArch:     "amd64",
	})
	if err != nil {
		t.Fatalf("MintInstallArtifact: %v", err)
	}
	if !strings.HasPrefix(artifact.PlaintextDownloadToken, enrollment.InstallPrefix) {
		t.Fatalf("download token missing dl_ prefix: %q", artifact.PlaintextDownloadToken)
	}

	// First curl: should get a minted PAT and a success outcome.
	res, err := svc.Svc.ConsumeInstallDownload(ctx, artifact.PlaintextDownloadToken,
		enrollment.ConsumeContext{ClientIP: "10.0.0.5", ClientUA: "curl/8.0"})
	if err != nil {
		t.Fatalf("ConsumeInstallDownload: %v", err)
	}
	if res.Outcome != "success" {
		t.Fatalf("Outcome = %q; want success", res.Outcome)
	}
	if res.PATPlaintext == "" || !strings.HasPrefix(res.PATPlaintext, enrollment.EnrollmentTokenPrefix) {
		t.Fatalf("PATPlaintext missing/malformed: %q", res.PATPlaintext)
	}
	if res.ServerEndpoint != "127.0.0.1:13337" {
		t.Fatalf("ServerEndpoint = %q", res.ServerEndpoint)
	}

	// End-to-end: the minted PAT redeems correctly into an agent session.
	redeem, err := svc.Svc.RedeemEnrollmentToken(ctx, res.PATPlaintext, enrollment.RedeemContext{
		ClientIP: "10.0.0.5", MachineID: "m1",
	})
	if err != nil || redeem.Outcome != "success" {
		t.Fatalf("RedeemEnrollmentToken of freshly minted: %v / %q", err, redeem.Outcome)
	}

	// Replay curl: the install token is consumed; a second attempt is logged
	// as already_consumed and no new PAT is issued.
	second, err := svc.Svc.ConsumeInstallDownload(ctx, artifact.PlaintextDownloadToken,
		enrollment.ConsumeContext{ClientIP: "attacker"})
	if err != nil {
		t.Fatalf("second ConsumeInstallDownload: %v", err)
	}
	if second.Outcome != "already_consumed" {
		t.Fatalf("second Outcome = %q; want already_consumed", second.Outcome)
	}
	if second.PATPlaintext != "" {
		t.Fatal("second call leaked a PAT plaintext")
	}

	// Audit: both attempts landed in install_download_events, first success
	// then already_consumed.
	events, err := svc.DB().InstallDownloadEvents().ListByDownload(ctx, artifact.DownloadID, 10)
	if err != nil {
		t.Fatalf("ListByDownload: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d; want 2", len(events))
	}
	var sawSuccess, sawReplay bool
	for _, e := range events {
		switch e.Outcome {
		case "success":
			sawSuccess = true
		case "already_consumed":
			sawReplay = true
		}
	}
	if !sawSuccess || !sawReplay {
		t.Fatalf("events didn't cover both outcomes: %+v", events)
	}
}

// BaselinePluginIDs survive the round trip from Mint → DB → Consume →
// caller. The list is what the agent will use to filter system plugins
// at boot, so any drop-on-the-floor here would silently re-enable the
// unrestricted boot mode.
func TestConsumeInstallDownload_PreservesBaselinePluginIDs(t *testing.T) {
	svc := newSvc(t)
	admin, proj := bootstrap(t, svc.DB())
	ctx := context.Background()

	artifact, err := svc.Svc.MintInstallArtifact(ctx, enrollment.MintInstallArtifactInput{
		ProjectID:         proj.ID,
		IssuedByUser:      admin.ID,
		ServerEndpoint:    "127.0.0.1:13337",
		BaselinePluginIDs: []string{"com.platypus.sys-info", "com.platypus.sys-listdir"},
	})
	if err != nil {
		t.Fatalf("MintInstallArtifact: %v", err)
	}

	res, err := svc.Svc.ConsumeInstallDownload(ctx, artifact.PlaintextDownloadToken,
		enrollment.ConsumeContext{ClientIP: "10.0.0.5"})
	if err != nil {
		t.Fatalf("ConsumeInstallDownload: %v", err)
	}
	if res.Outcome != "success" {
		t.Fatalf("Outcome = %q; want success", res.Outcome)
	}
	if len(res.BaselinePluginIDs) != 2 ||
		res.BaselinePluginIDs[0] != "com.platypus.sys-info" ||
		res.BaselinePluginIDs[1] != "com.platypus.sys-listdir" {
		t.Fatalf("BaselinePluginIDs = %v", res.BaselinePluginIDs)
	}
}

// Mint without BaselinePluginIDs leaves the consume result with nil
// (vs. []string{}) — the agent's bootstrap distinguishes "no allowlist
// set" from "explicit empty allowlist".
func TestConsumeInstallDownload_NoBaselineYieldsNil(t *testing.T) {
	svc := newSvc(t)
	admin, proj := bootstrap(t, svc.DB())
	ctx := context.Background()

	artifact, err := svc.Svc.MintInstallArtifact(ctx, enrollment.MintInstallArtifactInput{
		ProjectID: proj.ID, IssuedByUser: admin.ID, ServerEndpoint: "127.0.0.1:13337",
	})
	if err != nil {
		t.Fatalf("MintInstallArtifact: %v", err)
	}

	res, err := svc.Svc.ConsumeInstallDownload(ctx, artifact.PlaintextDownloadToken,
		enrollment.ConsumeContext{ClientIP: "10.0.0.5"})
	if err != nil {
		t.Fatalf("ConsumeInstallDownload: %v", err)
	}
	if res.BaselinePluginIDs != nil {
		t.Fatalf("BaselinePluginIDs = %v; want nil", res.BaselinePluginIDs)
	}
}

func TestConsumeInstallDownload_InvalidSecret_DoesNotMintEnrollmentToken(t *testing.T) {
	svc := newSvc(t)
	admin, proj := bootstrap(t, svc.DB())
	ctx := context.Background()

	artifact, err := svc.Svc.MintInstallArtifact(ctx, enrollment.MintInstallArtifactInput{
		ProjectID: proj.ID, IssuedByUser: admin.ID, ServerEndpoint: "127.0.0.1:13337",
	})
	if err != nil {
		t.Fatalf("MintInstallArtifact: %v", err)
	}

	// Keep the `dl_<id>.` prefix but swap the secret for a different but
	// well-formed base32 run. Using all-'a' works because base32 decodes
	// to zero bytes, guaranteed to mismatch the random secret.
	dot := strings.IndexByte(artifact.PlaintextDownloadToken, '.')
	bad := artifact.PlaintextDownloadToken[:dot+1] + "aaaaaaaaaaaaaaaaaaaa"
	res, err := svc.Svc.ConsumeInstallDownload(ctx, bad, enrollment.ConsumeContext{})
	if err != nil {
		t.Fatalf("ConsumeInstallDownload: %v", err)
	}
	if res.Outcome != "invalid_secret" {
		t.Fatalf("Outcome = %q; want invalid_secret", res.Outcome)
	}
	// The install token is still pending; a subsequent correct curl works.
	good, err := svc.Svc.ConsumeInstallDownload(ctx, artifact.PlaintextDownloadToken, enrollment.ConsumeContext{})
	if err != nil || good.Outcome != "success" {
		t.Fatalf("follow-up: %v / %q", err, good.Outcome)
	}
}

func TestConsumeInstallDownload_MalformedAndUnknown(t *testing.T) {
	svc := newSvc(t)
	_, _ = bootstrap(t, svc.DB())
	ctx := context.Background()

	if res, err := svc.Svc.ConsumeInstallDownload(ctx, "nope", enrollment.ConsumeContext{}); err != enrollment.ErrMalformed || res.Outcome != "malformed" {
		t.Fatalf("malformed: err=%v outcome=%q", err, res.Outcome)
	}
	res, err := svc.Svc.ConsumeInstallDownload(ctx, "dl_aaaaa.bbbbb", enrollment.ConsumeContext{})
	if err != nil {
		t.Fatalf("unknown_id: err=%v", err)
	}
	if res.Outcome != "unknown_id" {
		t.Fatalf("Outcome = %q; want unknown_id", res.Outcome)
	}
}

func TestConsumeInstallDownload_Expired(t *testing.T) {
	svc := newSvc(t)
	admin, proj := bootstrap(t, svc.DB())
	ctx := context.Background()

	// Negative TTL → already expired at mint. The default input path coerces
	// ttl<=0 to DefaultInstallTTL, so we go via a direct storage write.
	artifact, err := svc.Svc.MintInstallArtifact(ctx, enrollment.MintInstallArtifactInput{
		ProjectID: proj.ID, IssuedByUser: admin.ID, ServerEndpoint: "127.0.0.1:13337",
	})
	if err != nil {
		t.Fatalf("MintInstallArtifact: %v", err)
	}
	// Force-expire via direct UPDATE — a bit ugly but lets us test the
	// classification without waiting for 5 real minutes.
	_, err = svc.DB().Exec(
		`UPDATE install_download_tokens SET expires_at = ? WHERE download_id = ?`,
		time.Now().Add(-time.Hour).UTC(), artifact.DownloadID)
	if err != nil {
		t.Fatalf("force-expire: %v", err)
	}

	res, err := svc.Svc.ConsumeInstallDownload(ctx, artifact.PlaintextDownloadToken, enrollment.ConsumeContext{})
	if err != nil {
		t.Fatalf("ConsumeInstallDownload: %v", err)
	}
	if res.Outcome != "expired" {
		t.Fatalf("Outcome = %q; want expired", res.Outcome)
	}
}
