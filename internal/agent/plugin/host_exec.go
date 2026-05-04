package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"time"

	extism "github.com/extism/go-sdk"
)

// host_exec lets a plugin spawn one of an explicit command allowlist
// (manifest.capabilities.exec.commands) and capture its stdout +
// stderr. Capability gate: CapExec.
//
// Two enforcement layers: the operator-confirmed CapExec grant
// (catalog) and the manifest's per-command allowlist. Both must be
// present for the call to proceed.

type execRequest struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	CWD       string            `json:"cwd"`
	TimeoutMS uint32            `json:"timeout_ms"`
}

type execResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

func (pctx *pluginCtx) hostExec(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
	if !pctx.granted[CapExec] {
		returnEnvelope(p, stack, denied("exec"))
		return
	}
	raw, err := readStringArg(p, stack[0])
	if err != nil {
		returnEnvelope(p, stack, failed("read_request: "+err.Error()))
		return
	}
	var req execRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		returnEnvelope(p, stack, failed("decode_request: "+err.Error()))
		return
	}
	if pctx.manifest.Capabilities.Exec == nil {
		returnEnvelope(p, stack, denied("exec (no manifest spec)"))
		return
	}
	// "*" in the commands list is the unrestricted-exec marker. It's
	// only a sane choice for system plugins (which the operator
	// implicitly trusts via the agent build); the install-time
	// capability dialog should call out a "*" entry prominently for
	// third-party plugins so it doesn't get rubber-stamped.
	allowed := false
	for _, c := range pctx.manifest.Capabilities.Exec.Commands {
		if c == "*" || c == req.Command {
			allowed = true
			break
		}
	}
	if !allowed {
		returnEnvelope(p, stack, denied("command_not_in_allowlist: "+req.Command))
		return
	}

	cctx := ctx
	if req.TimeoutMS > 0 {
		var cancel context.CancelFunc
		cctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMS)*time.Millisecond)
		defer cancel()
	}
	cmd := exec.CommandContext(cctx, req.Command, req.Args...)
	cmd.Dir = req.CWD
	if len(req.Env) > 0 {
		env := make([]string, 0, len(req.Env))
		for k, v := range req.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}
	// Capture stdout + stderr into separate buffers. The previous
	// cmd.Output() form lost stderr on exit_code=0 because *only*
	// ExitError carries the .Stderr fallback; a successful run with
	// stderr writes (e.g. progress messages) silently dropped them.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	resp := execResponse{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		resp.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		returnEnvelope(p, stack, failed("exec: "+err.Error()))
		return
	}
	returnEnvelope(p, stack, okData(resp))
}
