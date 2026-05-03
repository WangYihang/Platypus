package bridge_test

import (
	"context"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// Smoke tests for the C7 bridges. The real check / auditor logic is
// covered in internal/agent/security and config_audit tests; here we
// only assert the plugin → host fn round-trip carries the response
// through cleanly.

func TestBridge_SecurityScan_RoundTripsStubProvider(t *testing.T) {
	plugin.SetHostSecurityScanProvider(func(_ context.Context, req *v2pb.SecurityScanRequest) *v2pb.SecurityScanResponse {
		return &v2pb.SecurityScanResponse{
			StartedAtUnix: 12345,
			ElapsedMs:     7,
			Findings: []*v2pb.SecurityFinding{
				{Id: "test.finding", Severity: "low", Title: "synthetic"},
			},
		}
	})
	t.Cleanup(func() { plugin.SetHostSecurityScanProvider(nil) })

	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.SecurityScan(reg)(context.Background(), &v2pb.SecurityScanRequest{})
	if resp.GetError() != "" {
		t.Fatalf("err: %s", resp.GetError())
	}
	if resp.GetStartedAtUnix() != 12345 {
		t.Errorf("started_at = %d", resp.GetStartedAtUnix())
	}
	if len(resp.GetFindings()) != 1 || resp.GetFindings()[0].GetId() != "test.finding" {
		t.Errorf("findings = %+v", resp.GetFindings())
	}
}

func TestBridge_ListSecurityChecks_RoundTripsStubProvider(t *testing.T) {
	plugin.SetHostListSecurityChecksProvider(func(_ context.Context, _ *v2pb.ListSecurityChecksRequest) *v2pb.ListSecurityChecksResponse {
		return &v2pb.ListSecurityChecksResponse{
			Checks: []*v2pb.AvailableSecurityCheck{
				{Id: "test.check", Category: "synthetic", Applicable: true},
			},
		}
	})
	t.Cleanup(func() { plugin.SetHostListSecurityChecksProvider(nil) })

	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.ListSecurityChecks(reg)(context.Background(), &v2pb.ListSecurityChecksRequest{})
	if len(resp.GetChecks()) != 1 || resp.GetChecks()[0].GetId() != "test.check" {
		t.Errorf("checks = %+v", resp.GetChecks())
	}
}
