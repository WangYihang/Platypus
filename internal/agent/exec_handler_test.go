package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleExec runs an ExecRequest on the agent host (os/exec-backed)
// and packages the stdout/stderr/exit_code into an ExecResponse.
// Errors starting the child (e.g. unknown binary) land in
// ExecResponse.Error; non-zero exit codes go in ExitCode without
// populating Error.

func TestHandleExec_StdoutCapture(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp := HandleExec(ctx, &v2pb.ExecRequest{
		Command: "printf",
		Args:    []string{"hello"},
	})
	if resp.Error != "" {
		t.Fatalf("resp.Error = %q; want empty", resp.Error)
	}
	if string(resp.Stdout) != "hello" {
		t.Fatalf("stdout = %q; want hello", resp.Stdout)
	}
	if resp.ExitCode != 0 {
		t.Fatalf("exit code = %d; want 0", resp.ExitCode)
	}
}

func TestHandleExec_NonZeroExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// `false` exits with status 1 on every POSIX system and is
	// present in any minimally-featured image, including alpine.
	resp := HandleExec(ctx, &v2pb.ExecRequest{Command: "false"})
	if resp.Error != "" {
		t.Fatalf("resp.Error should be empty for clean non-zero exit; got %q", resp.Error)
	}
	if resp.ExitCode == 0 {
		t.Fatal("expected non-zero exit code from `false`")
	}
}

func TestHandleExec_UnknownBinary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp := HandleExec(ctx, &v2pb.ExecRequest{
		Command: "/definitely/does/not/exist/platypus-test",
	})
	if resp.Error == "" {
		t.Fatal("expected resp.Error to be non-empty for missing binary")
	}
}

// Context cancellation kills the child and HandleExec returns
// promptly. Tests use a command that sleeps longer than the
// context.
func TestHandleExec_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	resp := HandleExec(ctx, &v2pb.ExecRequest{
		Command: "sleep",
		Args:    []string{"3"},
	})
	// On context kill we expect a non-zero exit or a populated
	// Error — exact shape depends on the OS's signal reporting.
	if resp.Error == "" && resp.ExitCode == 0 {
		t.Fatalf("expected non-zero exit or Error on ctx kill; got %+v", resp)
	}
}

// Env variables pass through.
func TestHandleExec_EnvPropagation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp := HandleExec(ctx, &v2pb.ExecRequest{
		Command: "sh",
		Args:    []string{"-c", "printf $PLATYPUS_EXEC_TEST"},
		Env:     map[string]string{"PLATYPUS_EXEC_TEST": "from_env"},
	})
	if resp.Error != "" {
		t.Fatalf("resp.Error = %q", resp.Error)
	}
	if !strings.Contains(string(resp.Stdout), "from_env") {
		t.Fatalf("env var did not propagate; stdout = %q", resp.Stdout)
	}
}
