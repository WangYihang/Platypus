package core

import (
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/enrollment"
)

// TestRenderInstallScript_PinsProjectCA locks the H1 fix in: when the
// server has a project CA, the rendered script must verify the artifact
// download against it (--cacert), not skip TLS verification.
func TestRenderInstallScript_PinsProjectCA(t *testing.T) {
	r := &enrollment.ConsumeResult{
		ServerEndpoint: "agent.example.com:13337",
		PATPlaintext:   "plt_id.secret",
		ProjectCAPEM:   "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n",
	}
	got := renderInstallScript(r, "distributor.example.com")

	if strings.Contains(got, "curl -fsSLk ") {
		t.Errorf("rendered script still uses 'curl -fsSLk' — TLS verification skipped\n%s", got)
	}
	if !strings.Contains(got, "PLATYPUS_PROJECT_CA=") {
		t.Errorf("script missing PLATYPUS_PROJECT_CA env var\n%s", got)
	}
	if !strings.Contains(got, "--cacert $CA_FILE") {
		t.Errorf("script does not pin curl to the project CA via --cacert\n%s", got)
	}
	if !strings.Contains(got, "base64 -d") {
		t.Errorf("script does not decode the embedded CA into a temp file\n%s", got)
	}
}

// TestRenderInstallScript_RefusesInsecureByDefault locks the no-CA
// branch: if no project CA is configured the script must NOT silently
// fall back to -k. Operators must opt in via PLATYPUS_INSECURE_DOWNLOAD=1.
func TestRenderInstallScript_RefusesInsecureByDefault(t *testing.T) {
	r := &enrollment.ConsumeResult{
		ServerEndpoint: "agent.example.com:13337",
		PATPlaintext:   "plt_id.secret",
		// ProjectCAPEM intentionally empty.
	}
	got := renderInstallScript(r, "distributor.example.com")

	if strings.Contains(got, "PLATYPUS_PROJECT_CA=") {
		t.Errorf("script unexpectedly exports PLATYPUS_PROJECT_CA when none was configured\n%s", got)
	}
	if !strings.Contains(got, "PLATYPUS_INSECURE_DOWNLOAD") {
		t.Errorf("script does not gate insecure download behind PLATYPUS_INSECURE_DOWNLOAD\n%s", got)
	}
	if !strings.Contains(got, "refusing to download agent binary without TLS trust anchor") {
		t.Errorf("script does not refuse insecure download by default\n%s", got)
	}
	// Sanity: the script must still exit before curl when neither CA nor
	// the override is set. Look for the explicit `exit 1` in the else branch.
	if !strings.Contains(got, "exit 1\nfi\n") {
		t.Errorf("script does not abort before reaching curl in the no-CA / no-override branch\n%s", got)
	}
}
