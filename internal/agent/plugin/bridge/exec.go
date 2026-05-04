package bridge

import (
	"context"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// execPluginIDs is the preference chain. Merged sys-process
// (post-merge, also owns the process_open stream) wins where the
// publisher staged it; sys-exec is the legacy fallback for
// production agents still on the read-only embed FS. Both ids
// expose the exec RPC with byte-identical wire shapes.
//
// Capability-wise the merge does NOT widen authorisation: the
// merged plugin declares both `exec` and `process` independently,
// and the host's runtime check still gates each call separately.
var execPluginIDs = []string{
	"com.platypus.sys-process",
	"com.platypus.sys-exec",
}

// Exec is the plugin-backed replacement for agent.HandleExec.
//
// stdout / stderr come back as JSON strings; the legacy handler used
// raw bytes. We round-trip via string-encoded bytes here, which is
// correct for UTF-8 output and lossy only for arbitrary binary
// stdout (rare for agent-launched commands but worth noting).
func Exec(reg *plugin.Registry) func(ctx context.Context, req *v2pb.ExecRequest) *v2pb.ExecResponse {
	return func(ctx context.Context, req *v2pb.ExecRequest) *v2pb.ExecResponse {
		envCopy := map[string]string{}
		for k, v := range req.GetEnv() {
			envCopy[k] = v
		}
		var resp execJSONResponse
		pluginErr, err := invokeJSONFallback(ctx, reg, execPluginIDs, "exec", execJSONRequest{
			Command:   req.GetCommand(),
			Args:      req.GetArgs(),
			Env:       envCopy,
			CWD:       req.GetCwd(),
			TimeoutMS: req.GetTimeoutMs(),
		}, &resp)
		if err != nil {
			return &v2pb.ExecResponse{Error: "bridge: " + err.Error()}
		}
		if pluginErr != "" {
			return &v2pb.ExecResponse{Error: pluginErr}
		}
		return &v2pb.ExecResponse{
			Stdout:   []byte(resp.Stdout),
			Stderr:   []byte(resp.Stderr),
			ExitCode: resp.ExitCode,
			Error:    resp.Error,
		}
	}
}

type execJSONRequest struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	CWD       string            `json:"cwd,omitempty"`
	TimeoutMS uint32            `json:"timeout_ms,omitempty"`
}

type execJSONResponse struct {
	ExitCode int32  `json:"exit_code,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
	Error    string `json:"error,omitempty"`
}
