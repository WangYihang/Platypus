package bridge_test

import (
	"context"
	"strings"
	"testing"

	"github.com/WangYihang/Platypus/internal/agent/plugin/bridge"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func TestBridge_Exec_RoundsTripsStdout(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.Exec(reg)(context.Background(), &v2pb.ExecRequest{
		Command:   "/bin/echo",
		Args:      []string{"hello", "from", "plugin"},
		TimeoutMs: 5000,
	})
	if resp.GetError() != "" {
		t.Fatalf("exec err: %s", resp.GetError())
	}
	if resp.GetExitCode() != 0 {
		t.Errorf("exit_code = %d, want 0", resp.GetExitCode())
	}
	if got := strings.TrimSpace(string(resp.GetStdout())); got != "hello from plugin" {
		t.Errorf("stdout = %q", got)
	}
}

func TestBridge_Exec_NonzeroExit(t *testing.T) {
	reg := newRegWithSysPlugins(t)
	defer reg.Close(context.Background())

	resp := bridge.Exec(reg)(context.Background(), &v2pb.ExecRequest{
		Command:   "/bin/sh",
		Args:      []string{"-c", "exit 7"},
		TimeoutMs: 5000,
	})
	if resp.GetExitCode() != 7 {
		t.Errorf("exit_code = %d, want 7", resp.GetExitCode())
	}
}
