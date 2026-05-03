package bridge_test

import (
	"context"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func TestBridge_ConfigAudit_RoundTripsStubProvider(t *testing.T) {
	plugin.SetHostConfigAuditProvider(func(_ context.Context, _ *v2pb.ConfigAuditRequest) *v2pb.ConfigAuditResponse {
		return &v2pb.ConfigAuditResponse{
			StartedAtUnix: 22222,
			ElapsedMs:     11,
			Leaks: []*v2pb.ConfigLeak{
				{Id: "test.leak", Risk: "high", Title: "synthetic"},
			},
		}
	})
	t.Cleanup(func() { plugin.SetHostConfigAuditProvider(nil) })

	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.ConfigAudit(reg)(context.Background(), &v2pb.ConfigAuditRequest{})
	if resp.GetError() != "" {
		t.Fatalf("err: %s", resp.GetError())
	}
	if len(resp.GetLeaks()) != 1 || resp.GetLeaks()[0].GetId() != "test.leak" {
		t.Errorf("leaks = %+v", resp.GetLeaks())
	}
}

func TestBridge_ListConfigAuditors_RoundTripsStubProvider(t *testing.T) {
	plugin.SetHostListConfigAuditorsProvider(func(_ context.Context, _ *v2pb.ListConfigAuditorsRequest) *v2pb.ListConfigAuditorsResponse {
		return &v2pb.ListConfigAuditorsResponse{
			Auditors: []*v2pb.AvailableAuditor{
				{Id: "test.aud", Category: "synthetic", Applicable: true},
			},
		}
	})
	t.Cleanup(func() { plugin.SetHostListConfigAuditorsProvider(nil) })

	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.ListConfigAuditors(reg)(context.Background(), &v2pb.ListConfigAuditorsRequest{})
	if len(resp.GetAuditors()) != 1 || resp.GetAuditors()[0].GetId() != "test.aud" {
		t.Errorf("auditors = %+v", resp.GetAuditors())
	}
}
