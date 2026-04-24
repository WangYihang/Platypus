package core_test

import (
	"testing"

	"github.com/WangYihang/Platypus/internal/core"
)

// NewAgentService + getters round-trip their config without surprises.
// Also pins that a nil service is a safe no-op for the Snapshot / Addr
// / ProjectID accessors so callers on the legacy path don't have to
// guard every read with a nil check.
func TestAgentService_ConfigAccessors(t *testing.T) {
	svc := core.NewAgentService(core.AgentServiceConfig{
		HashFormat:  "%i %u",
		ShellPath:   "/bin/zsh",
		IngressAddr: "0.0.0.0:9443",
		ProjectID:   "proj-1",
	})
	if got := svc.IngressAddr(); got != "0.0.0.0:9443" {
		t.Errorf("IngressAddr=%q; want 0.0.0.0:9443", got)
	}
	if got := svc.ProjectID(); got != "proj-1" {
		t.Errorf("ProjectID=%q; want proj-1", got)
	}
	if snap := svc.Snapshot(); len(snap) != 0 {
		t.Errorf("fresh Snapshot = %v; want empty map", snap)
	}

	var nilSvc *core.AgentService
	if nilSvc.IngressAddr() != "" || nilSvc.ProjectID() != "" || nilSvc.Snapshot() != nil {
		t.Error("nil AgentService accessors should be no-op")
	}
}

// SetAgentService / Agents is the injection seam main.go uses. Round-
// tripping it — including clearing it back to nil so other tests can
// rely on a clean slate — is part of the contract.
func TestAgentService_Register(t *testing.T) {
	svc := core.NewAgentService(core.AgentServiceConfig{IngressAddr: "1.2.3.4:9443"})
	core.SetAgentService(svc)
	t.Cleanup(func() { core.SetAgentService(nil) })

	got := core.Agents()
	if got != svc {
		t.Fatalf("Agents() = %p; want %p", got, svc)
	}
	if got.IngressAddr() != "1.2.3.4:9443" {
		t.Errorf("ingress leak: %q", got.IngressAddr())
	}
}
