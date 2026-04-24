package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleExec runs a single ExecRequest and returns the result. It
// is the production Exec handler the agent registers with
// AgentRPCHandlers.Exec; tests call it directly.
//
// Behaviour contract (pinned by unit tests):
//
//   - Non-existent binary → Error non-empty, ExitCode 0.
//   - Child exits non-zero cleanly → Error empty, ExitCode non-zero.
//     Callers distinguish "RPC failed on the wire" from "command
//     ran and returned non-zero".
//   - Context cancellation → child is killed and the function returns
//     promptly; ExitCode / Error reflect whatever the OS delivers.
//   - TimeoutMs (if non-zero) bounds the total command run on top of
//     any wider context deadline the caller already set.
func HandleExec(ctx context.Context, req *v2pb.ExecRequest) *v2pb.ExecResponse {
	if req == nil {
		return &v2pb.ExecResponse{Error: "agent: HandleExec: nil request"}
	}
	if req.Command == "" {
		return &v2pb.ExecResponse{Error: "agent: HandleExec: empty command"}
	}

	if req.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	// Inherit the agent's environment and merge in caller-supplied
	// entries. Merge order matters: user-supplied wins so PATH / LANG
	// etc. can be overridden per-call.
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), flattenEnv(req.Env)...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	resp := &v2pb.ExecResponse{}
	err := cmd.Run()
	resp.Stdout = stdout.Bytes()
	resp.Stderr = stderr.Bytes()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Clean non-zero exit — not an Error, caller inspects
			// ExitCode.
			resp.ExitCode = int32(exitErr.ExitCode())
			return resp
		}
		// Spawn failure (binary not found, permission denied) or
		// ctx-triggered kill surfaces here.
		resp.Error = err.Error()
		return resp
	}
	resp.ExitCode = 0
	return resp
}

// flattenEnv turns a map into the KEY=VALUE slice os/exec wants.
func flattenEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
