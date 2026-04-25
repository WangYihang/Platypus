package agent

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/WangYihang/Platypus/internal/link"
	"github.com/WangYihang/Platypus/internal/log"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// AgentRPCHandlers is the per-payload-type registry. Each field is
// a function that takes the specific payload and returns the
// matching response. Nil handlers produce an RpcResponse.Error of
// "unsupported" so clients can distinguish "server-doesn't-
// implement-this-yet" from "handler ran and failed".
//
// Nil handlers are the default: agents only wire the handlers for
// RPCs they actually support. Adding a new RPC = adding a new
// field here and a new case in the dispatch switch below.
type AgentRPCHandlers struct {
	Exec        func(ctx context.Context, req *v2pb.ExecRequest) *v2pb.ExecResponse
	ListDir     func(ctx context.Context, req *v2pb.ListDirRequest) *v2pb.ListDirResponse
	Stat        func(ctx context.Context, req *v2pb.StatRequest) *v2pb.StatResponse
	Delete      func(ctx context.Context, req *v2pb.DeleteRequest) *v2pb.DeleteResponse
	Rename      func(ctx context.Context, req *v2pb.RenameRequest) *v2pb.RenameResponse
	Mkdir       func(ctx context.Context, req *v2pb.MkdirRequest) *v2pb.MkdirResponse
	Chmod       func(ctx context.Context, req *v2pb.ChmodRequest) *v2pb.ChmodResponse
	SysInfo     func(ctx context.Context, req *v2pb.SysInfoRequest) *v2pb.SysInfoResponse
	ProcessList func(ctx context.Context, req *v2pb.ProcessListRequest) *v2pb.ProcessListResponse
}

// correlationKey is the context key under which ServeRPCStream (or
// its caller) stashes the corr_id read off StreamHeader. It lets
// dispatchRPC include the same id in its log lines without
// widening any function signatures. Unset / empty is fine and just
// shows up as corr_id="" in the logs.
type correlationKey struct{}

// ContextWithCorrelationID returns a copy of ctx carrying corrID
// for later retrieval by dispatchRPC. Exported so the agent's
// per-stream router (dispatchAgentStream) can seed the value
// straight from StreamHeader.
func ContextWithCorrelationID(ctx context.Context, corrID string) context.Context {
	return context.WithValue(ctx, correlationKey{}, corrID)
}

// correlationFromContext is the reader side; returns "" when unset.
func correlationFromContext(ctx context.Context) string {
	v, _ := ctx.Value(correlationKey{}).(string)
	return v
}

// ServeRPCStream is the agent-side entrypoint for a single accepted
// STREAM_TYPE_RPC stream: read one RpcRequest, dispatch to the
// matching field of handlers, write one RpcResponse, close.
// Returns any wire-level error so the outer stream dispatcher can
// log it; service-level errors are carried inside RpcResponse.Error
// (distinguishable from wire failures).
func ServeRPCStream(ctx context.Context, stream io.ReadWriteCloser, handlers AgentRPCHandlers) error {
	defer func() { _ = stream.Close() }()

	var req v2pb.RpcRequest
	if err := link.ReadFrame(stream, &req); err != nil {
		return fmt.Errorf("agent: ServeRPCStream read: %w", err)
	}
	resp := dispatchRPC(ctx, &req, handlers)
	if err := link.WriteFrame(stream, resp); err != nil {
		return fmt.Errorf("agent: ServeRPCStream write: %w", err)
	}
	return nil
}

// dispatchRPC wraps dispatchRPCInner with structured start / end
// logs. The split keeps the pure routing logic unit-testable while
// still covering every case arm with a single log pair. Fields
// mirror the server-side agent_rpc_* lines so operators can match
// request/response across log streams by (agent_id, type, rough
// timestamp) — the corr_id isn't wired through the link today, but
// gets populated here if the caller stashed one in ctx.
func dispatchRPC(ctx context.Context, req *v2pb.RpcRequest, h AgentRPCHandlers) *v2pb.RpcResponse {
	start := time.Now()
	name := rpcPayloadName(req.GetPayload())
	corrID := correlationFromContext(ctx)
	log.L.Info("rpc_dispatch_start",
		"type", name,
		"corr_id", corrID,
	)
	resp := dispatchRPCInner(ctx, req, h)
	elapsed := time.Since(start).Milliseconds()
	switch {
	case resp == nil:
		log.L.Warn("rpc_dispatch_nil_resp",
			"type", name,
			"corr_id", corrID,
			"elapsed_ms", elapsed,
		)
	case resp.GetError() != "":
		log.L.Warn("rpc_dispatch_app_error",
			"type", name,
			"corr_id", corrID,
			"elapsed_ms", elapsed,
			"error", resp.GetError(),
		)
	default:
		log.L.Info("rpc_dispatch_ok",
			"type", name,
			"corr_id", corrID,
			"elapsed_ms", elapsed,
		)
	}
	return resp
}

// dispatchRPCInner is the pure routing logic: pick the handler for
// the request's payload type, invoke it, wrap the result into an
// RpcResponse. Separated so unit tests can exercise the dispatch
// logic without a real io.ReadWriteCloser.
func dispatchRPCInner(ctx context.Context, req *v2pb.RpcRequest, h AgentRPCHandlers) *v2pb.RpcResponse {
	switch p := req.Payload.(type) {
	case *v2pb.RpcRequest_Exec:
		if h.Exec == nil {
			return unsupported("exec")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Exec{Exec: h.Exec(ctx, p.Exec)}}
	case *v2pb.RpcRequest_ListDir:
		if h.ListDir == nil {
			return unsupported("list_dir")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_ListDir{ListDir: h.ListDir(ctx, p.ListDir)}}
	case *v2pb.RpcRequest_Stat:
		if h.Stat == nil {
			return unsupported("stat")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Stat{Stat: h.Stat(ctx, p.Stat)}}
	case *v2pb.RpcRequest_Delete:
		if h.Delete == nil {
			return unsupported("delete")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Delete{Delete: h.Delete(ctx, p.Delete)}}
	case *v2pb.RpcRequest_Rename:
		if h.Rename == nil {
			return unsupported("rename")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Rename{Rename: h.Rename(ctx, p.Rename)}}
	case *v2pb.RpcRequest_Mkdir:
		if h.Mkdir == nil {
			return unsupported("mkdir")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Mkdir{Mkdir: h.Mkdir(ctx, p.Mkdir)}}
	case *v2pb.RpcRequest_Chmod:
		if h.Chmod == nil {
			return unsupported("chmod")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_Chmod{Chmod: h.Chmod(ctx, p.Chmod)}}
	case *v2pb.RpcRequest_SysInfo:
		if h.SysInfo == nil {
			return unsupported("sys_info")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_SysInfo{SysInfo: h.SysInfo(ctx, p.SysInfo)}}
	case *v2pb.RpcRequest_ProcessList:
		if h.ProcessList == nil {
			return unsupported("process_list")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_ProcessList{ProcessList: h.ProcessList(ctx, p.ProcessList)}}
	default:
		return &v2pb.RpcResponse{Error: "agent: unknown RPC payload type"}
	}
}

// rpcPayloadName returns a stable short string for one of the
// oneof variants carried by RpcRequest/RpcResponse. Intentionally
// duplicated (tiny) with internal/core.rpcPayloadName so neither
// package gains a cross-dependency just for a log helper.
func rpcPayloadName(p any) string {
	switch p.(type) {
	case *v2pb.RpcRequest_Exec, *v2pb.RpcResponse_Exec:
		return "exec"
	case *v2pb.RpcRequest_ListDir, *v2pb.RpcResponse_ListDir:
		return "list_dir"
	case *v2pb.RpcRequest_Stat, *v2pb.RpcResponse_Stat:
		return "stat"
	case *v2pb.RpcRequest_Delete, *v2pb.RpcResponse_Delete:
		return "delete"
	case *v2pb.RpcRequest_Rename, *v2pb.RpcResponse_Rename:
		return "rename"
	case *v2pb.RpcRequest_Mkdir, *v2pb.RpcResponse_Mkdir:
		return "mkdir"
	case *v2pb.RpcRequest_Chmod, *v2pb.RpcResponse_Chmod:
		return "chmod"
	case *v2pb.RpcRequest_SysInfo, *v2pb.RpcResponse_SysInfo:
		return "sys_info"
	case *v2pb.RpcRequest_ProcessList, *v2pb.RpcResponse_ProcessList:
		return "process_list"
	case nil:
		return ""
	default:
		return "unknown"
	}
}

func unsupported(name string) *v2pb.RpcResponse {
	return &v2pb.RpcResponse{Error: "agent: RPC not supported by this agent: " + name}
}
