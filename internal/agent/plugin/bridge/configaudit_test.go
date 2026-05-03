package bridge_test

import (
	"context"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// TestBridge_ListConfigAuditors_RealPluginEnumerates exercises the
// v2 sys-config-audit plugin's auditor registry. The legacy host-fn-
// backed gitleaks integration is gone; the plugin owns the ruleset
// (smaller in v2 — shell.history + cloud.aws + ssh.private_keys).
func TestBridge_ListConfigAuditors_RealPluginEnumerates(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.ListConfigAuditors(reg)(context.Background(), &v2pb.ListConfigAuditorsRequest{})
	if resp.GetError() != "" {
		t.Fatalf("err: %s", resp.GetError())
	}
	got := map[string]bool{}
	for _, a := range resp.GetAuditors() {
		got[a.GetId()] = true
	}
	for _, id := range []string{"shell.history", "cloud.aws", "ssh.private_keys"} {
		if !got[id] {
			t.Errorf("missing auditor %q in %+v", id, got)
		}
	}
}

// TestBridge_ConfigAudit_RealPluginRuns drives a no-filter scan on
// the test host. The result depends on /home + /root contents; we
// only assert the response decoded + each auditor reports a status.
func TestBridge_ConfigAudit_RealPluginRuns(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.ConfigAudit(reg)(context.Background(), &v2pb.ConfigAuditRequest{})
	if resp.GetError() != "" {
		t.Fatalf("err: %s", resp.GetError())
	}
	if len(resp.GetAuditors()) == 0 {
		t.Errorf("expected per-auditor results")
	}
}
