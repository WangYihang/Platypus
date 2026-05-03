package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// HandleMgmt is the agent-side dispatcher for STREAM_TYPE_PLUGIN_MGMT.
// Bound into AgentHandlerDeps.PluginMgmt at startup; the StreamHeader
// metadata is unmarshalled into req by the link layer before this is
// called.
//
// Stream lifecycle by op:
//   - install: agent emits a series of PluginInstallProgress frames
//     until a terminal PHASE_INSTALLED / PHASE_FAILED is written. The
//     stream stays open across the whole install so the operator UI
//     can render a live progress bar.
//   - uninstall / list / enable / get_logs: agent writes one
//     PluginMgmtResponse and closes.
func (r *Registry) HandleMgmt(ctx context.Context, stream io.ReadWriteCloser, req *v2pb.PluginMgmtRequest) error {
	defer func() { _ = stream.Close() }()

	switch op := req.GetOp().(type) {
	case *v2pb.PluginMgmtRequest_Install:
		return r.handleInstall(ctx, stream, op.Install)
	case *v2pb.PluginMgmtRequest_Uninstall:
		return r.replyUninstall(stream, op.Uninstall)
	case *v2pb.PluginMgmtRequest_List:
		return r.replyList(stream)
	case *v2pb.PluginMgmtRequest_Enable:
		return r.replyEnable(stream, op.Enable)
	case *v2pb.PluginMgmtRequest_GetLogs:
		return r.replyGetLogs(stream, op.GetLogs)
	default:
		return writeMgmtErr(stream, "malformed_metadata: PluginMgmtRequest.op is empty")
	}
}

// writeMgmtErr writes a PluginMgmtResponse with only `error` set. Used
// for top-level rejections (unknown op, missing argument); per-op
// failures are encoded inside the per-op response variant.
func writeMgmtErr(stream io.Writer, msg string) error {
	resp := &v2pb.PluginMgmtResponse{Error: msg}
	if err := link.WriteFrame(stream, resp); err != nil {
		return fmt.Errorf("plugin: write mgmt err: %w", err)
	}
	return errors.New(msg)
}

// emitInstallProgress is the per-frame helper used during install. It
// drops the error on the floor — the install path treats progress
// frames as best-effort because losing one only affects the UI's
// progress bar, not the install correctness.
func emitInstallProgress(stream io.Writer, p *v2pb.PluginInstallProgress) {
	if err := link.WriteFrame(stream, p); err != nil {
		log.L.Debug("plugin.mgmt.progress_write_failed", "error", err.Error())
	}
}

// readChunk pulls one PluginInstallChunk off the stream during inline
// install. EOF before a `last=true` frame is treated as a transport
// failure (returned as error); per-frame parse errors return a
// proto.UnmarshalError-wrapped value so the caller can decide whether
// to translate into a PHASE_FAILED.
func readChunk(stream io.Reader) (*v2pb.PluginInstallChunk, error) {
	var c v2pb.PluginInstallChunk
	if err := link.ReadFrame(stream, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ---- list --------------------------------------------------------

func (r *Registry) replyList(stream io.Writer) error {
	resp := &v2pb.PluginMgmtResponse{
		Result: &v2pb.PluginMgmtResponse_List{
			List: &v2pb.PluginListResponse{Plugins: r.List()},
		},
	}
	if err := link.WriteFrame(stream, resp); err != nil {
		return fmt.Errorf("plugin: write list resp: %w", err)
	}
	return nil
}

// ---- uninstall ---------------------------------------------------

func (r *Registry) replyUninstall(stream io.Writer, req *v2pb.PluginUninstallRequest) error {
	out := &v2pb.PluginUninstallResponse{}
	if req.GetPluginId() == "" {
		out.Error = "plugin_id is required"
	} else if err := r.Remove(context.Background(), req.GetPluginId(), req.GetPurgeState()); err != nil {
		out.Error = err.Error()
	} else {
		log.L.Info("plugin.uninstall.ok",
			"plugin_id", req.GetPluginId(),
			"purge_state", req.GetPurgeState(),
			"actor", req.GetActor(),
		)
	}
	resp := &v2pb.PluginMgmtResponse{Result: &v2pb.PluginMgmtResponse_Uninstall{Uninstall: out}}
	if err := link.WriteFrame(stream, resp); err != nil {
		return fmt.Errorf("plugin: write uninstall resp: %w", err)
	}
	return nil
}

// ---- enable / disable --------------------------------------------

func (r *Registry) replyEnable(stream io.Writer, req *v2pb.PluginEnableRequest) error {
	out := &v2pb.PluginEnableResponse{}
	if req.GetPluginId() == "" {
		out.Error = "plugin_id is required"
	} else if err := r.SetEnabled(req.GetPluginId(), req.GetEnabled()); err != nil {
		out.Error = err.Error()
	} else {
		log.L.Info("plugin.enable.ok",
			"plugin_id", req.GetPluginId(),
			"enabled", req.GetEnabled(),
			"actor", req.GetActor(),
		)
	}
	resp := &v2pb.PluginMgmtResponse{Result: &v2pb.PluginMgmtResponse_Enable{Enable: out}}
	if err := link.WriteFrame(stream, resp); err != nil {
		return fmt.Errorf("plugin: write enable resp: %w", err)
	}
	return nil
}

// ---- get_logs ----------------------------------------------------

func (r *Registry) replyGetLogs(stream io.Writer, req *v2pb.PluginGetLogsRequest) error {
	out := &v2pb.PluginGetLogsResponse{}
	if req.GetPluginId() == "" {
		out.Error = "plugin_id is required"
	} else {
		entries, err := r.Tail(req.GetPluginId(), int(req.GetTailLines()))
		if errors.Is(err, os.ErrNotExist) {
			out.Error = "plugin_not_installed: " + req.GetPluginId()
		} else if err != nil {
			out.Error = err.Error()
		} else {
			out.Entries = entries
		}
	}
	resp := &v2pb.PluginMgmtResponse{Result: &v2pb.PluginMgmtResponse_GetLogs{GetLogs: out}}
	if err := link.WriteFrame(stream, resp); err != nil {
		return fmt.Errorf("plugin: write get_logs resp: %w", err)
	}
	return nil
}

