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
	Exec               func(ctx context.Context, req *v2pb.ExecRequest) *v2pb.ExecResponse
	ListDir            func(ctx context.Context, req *v2pb.ListDirRequest) *v2pb.ListDirResponse
	Stat               func(ctx context.Context, req *v2pb.StatRequest) *v2pb.StatResponse
	Delete             func(ctx context.Context, req *v2pb.DeleteRequest) *v2pb.DeleteResponse
	Rename             func(ctx context.Context, req *v2pb.RenameRequest) *v2pb.RenameResponse
	Mkdir              func(ctx context.Context, req *v2pb.MkdirRequest) *v2pb.MkdirResponse
	Chmod              func(ctx context.Context, req *v2pb.ChmodRequest) *v2pb.ChmodResponse
	SysInfo            func(ctx context.Context, req *v2pb.SysInfoRequest) *v2pb.SysInfoResponse
	ProcessList        func(ctx context.Context, req *v2pb.ProcessListRequest) *v2pb.ProcessListResponse
	SecurityScan       func(ctx context.Context, req *v2pb.SecurityScanRequest) *v2pb.SecurityScanResponse
	ListSecurityChecks func(ctx context.Context, req *v2pb.ListSecurityChecksRequest) *v2pb.ListSecurityChecksResponse
	ConfigAudit        func(ctx context.Context, req *v2pb.ConfigAuditRequest) *v2pb.ConfigAuditResponse
	ListConfigAuditors func(ctx context.Context, req *v2pb.ListConfigAuditorsRequest) *v2pb.ListConfigAuditorsResponse

	// PluginCall is the single dispatch hook for the plugin system:
	// the registry routes by plugin_id+method, so adding a plugin
	// never requires another field here. nil means the agent was
	// built without the plugin runtime wired in.
	PluginCall func(ctx context.Context, req *v2pb.PluginCallRequest) *v2pb.PluginCallResponse
}

// streamCtxKey carries per-stream identifiers (correlation_id,
// link_session_id) so dispatchRPC can include them in log lines
// without widening any function signatures. All fields are unset /
// empty when the StreamHeader didn't carry the matching value.
type streamCtxKey struct{}

type streamCtx struct {
	correlationID string
	linkSessionID string
}

// ContextWithStreamIDs returns a copy of ctx tagged with the per-stream
// identifiers parsed off StreamHeader. dispatchAgentStream calls this
// once per accepted stream so RPC handlers and any logs they emit can
// echo the same values the server stamped on the wire.
func ContextWithStreamIDs(ctx context.Context, correlationID, linkSessionID string) context.Context {
	return context.WithValue(ctx, streamCtxKey{}, streamCtx{
		correlationID: correlationID,
		linkSessionID: linkSessionID,
	})
}

// streamIDsFromContext is the reader; both fields default to "" when
// the context wasn't seeded.
func streamIDsFromContext(ctx context.Context) (correlationID, linkSessionID string) {
	if v, ok := ctx.Value(streamCtxKey{}).(streamCtx); ok {
		return v.correlationID, v.linkSessionID
	}
	return "", ""
}

// CorrelationIDFromContext exposes just the correlation id for handlers
// that want to thread it into sub-spans (e.g. process_list emitting
// its own enumerate / completion lines under the same id).
func CorrelationIDFromContext(ctx context.Context) string {
	c, _ := streamIDsFromContext(ctx)
	return c
}

// LinkSessionIDFromContext is the matching getter for the per-link id.
func LinkSessionIDFromContext(ctx context.Context) string {
	_, l := streamIDsFromContext(ctx)
	return l
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
// still covering every case arm with a single log pair.
//
// Events emitted (`rpc.serve.*` namespace mirrors the server-side
// `rpc.call.*`):
//
//	rpc.serve.start          -> always, before handler runs
//	rpc.serve.ok             -> handler returned a clean response
//	rpc.serve.app_error      -> handler returned non-empty Error
//	rpc.serve.empty_response -> handler returned nil response
//
// The emitted `correlation_id` and `link_session_id` come straight
// from StreamHeader, so server-side and agent-side log lines for one
// round-trip share both ids and a single grep ties them together.
func dispatchRPC(ctx context.Context, req *v2pb.RpcRequest, h AgentRPCHandlers) *v2pb.RpcResponse {
	start := time.Now()
	method := log.RPCMethodName(req.GetPayload())
	correlationID, linkSessionID := streamIDsFromContext(ctx)
	requestAttr := log.RPCRequestAttr(req.GetPayload())

	baseFields := []any{
		"link_session_id", linkSessionID,
		"correlation_id", correlationID,
		"rpc_method", method,
		requestAttr,
	}

	log.L.Info("rpc.serve.start", baseFields...)
	resp := dispatchRPCInner(ctx, req, h)
	elapsed := time.Since(start).Milliseconds()
	switch {
	case resp == nil:
		log.L.Warn("rpc.serve.empty_response", append(baseFields, "elapsed_ms", elapsed)...)
	case resp.GetError() != "":
		log.L.Warn("rpc.serve.app_error", append(baseFields,
			"elapsed_ms", elapsed,
			"error", resp.GetError(),
		)...)
	default:
		log.L.Info("rpc.serve.ok", append(baseFields,
			"elapsed_ms", elapsed,
			log.RPCResponseAttr(resp.GetPayload()),
		)...)
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
	case *v2pb.RpcRequest_SecurityScan:
		if h.SecurityScan == nil {
			return unsupported("security_scan")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_SecurityScan{SecurityScan: h.SecurityScan(ctx, p.SecurityScan)}}
	case *v2pb.RpcRequest_ListSecurityChecks:
		if h.ListSecurityChecks == nil {
			return unsupported("list_security_checks")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_ListSecurityChecks{ListSecurityChecks: h.ListSecurityChecks(ctx, p.ListSecurityChecks)}}
	case *v2pb.RpcRequest_ConfigAudit:
		if h.ConfigAudit == nil {
			return unsupported("config_audit")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_ConfigAudit{ConfigAudit: h.ConfigAudit(ctx, p.ConfigAudit)}}
	case *v2pb.RpcRequest_ListConfigAuditors:
		if h.ListConfigAuditors == nil {
			return unsupported("list_config_auditors")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_ListConfigAuditors{ListConfigAuditors: h.ListConfigAuditors(ctx, p.ListConfigAuditors)}}
	case *v2pb.RpcRequest_PluginCall:
		if h.PluginCall == nil {
			return unsupported("plugin_call")
		}
		return &v2pb.RpcResponse{Payload: &v2pb.RpcResponse_PluginCall{PluginCall: h.PluginCall(ctx, p.PluginCall)}}
	default:
		return &v2pb.RpcResponse{Error: "agent: unknown RPC payload type"}
	}
}

func unsupported(name string) *v2pb.RpcResponse {
	return &v2pb.RpcResponse{Error: "agent: RPC not supported by this agent: " + name}
}
