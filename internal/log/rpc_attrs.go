package log

import (
	"fmt"
	"log/slog"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// RPCMethodName returns the canonical short name for one of the
// RpcRequest / RpcResponse oneof variants. Used as the
// `rpc_method` log field across both server and agent so a single
// string identifies the call regardless of which side is logging.
// nil collapses to "" and an unrecognised concrete type to "unknown"
// so log consumers can still filter for them.
func RPCMethodName(payload any) string {
	switch payload.(type) {
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
	case *v2pb.RpcRequest_SecurityScan, *v2pb.RpcResponse_SecurityScan:
		return "security_scan"
	case nil:
		return ""
	default:
		return "unknown"
	}
}

// RPCRequestAttr returns a `request` slog.Group describing the
// notable fields of an RpcRequest payload — what an operator
// reaching for the logs wants to see (paths, modes, top_n, command).
// Bulk content (exec.args, exec.env, file bodies) is summarised by
// count rather than logged verbatim so secrets in env or argv don't
// leak into the audit stream. nil / unknown payloads return an empty
// group, which slog elides from the rendered output — error-path log
// lines therefore omit the `request` field entirely rather than
// producing `request: {}`. Same pattern applies to RPCResponseAttr.
func RPCRequestAttr(payload any) slog.Attr {
	switch r := payload.(type) {
	case *v2pb.RpcRequest_Exec:
		e := r.Exec
		return slog.Group("request",
			"command", e.GetCommand(),
			"arg_count", len(e.GetArgs()),
			"env_count", len(e.GetEnv()),
			"cwd", e.GetCwd(),
			"timeout_ms", e.GetTimeoutMs(),
		)
	case *v2pb.RpcRequest_ListDir:
		return slog.Group("request", "path", r.ListDir.GetPath())
	case *v2pb.RpcRequest_Stat:
		return slog.Group("request", "path", r.Stat.GetPath())
	case *v2pb.RpcRequest_Delete:
		return slog.Group("request",
			"path", r.Delete.GetPath(),
			"recursive", r.Delete.GetRecursive(),
		)
	case *v2pb.RpcRequest_Rename:
		return slog.Group("request",
			"from", r.Rename.GetFrom(),
			"to", r.Rename.GetTo(),
		)
	case *v2pb.RpcRequest_Mkdir:
		m := r.Mkdir
		return slog.Group("request",
			"path", m.GetPath(),
			"mode_octal", fmt.Sprintf("%04o", m.GetMode()),
			"mkdirs", m.GetMkdirs(),
		)
	case *v2pb.RpcRequest_Chmod:
		c := r.Chmod
		return slog.Group("request",
			"path", c.GetPath(),
			"mode_octal", fmt.Sprintf("%04o", c.GetMode()),
		)
	case *v2pb.RpcRequest_SysInfo:
		return slog.Group("request")
	case *v2pb.RpcRequest_ProcessList:
		p := r.ProcessList
		return slog.Group("request",
			"top_n", p.GetTopN(),
			"sort_by", p.GetSortBy(),
		)
	case *v2pb.RpcRequest_SecurityScan:
		s := r.SecurityScan
		return slog.Group("request",
			"check_id_count", len(s.GetCheckIds()),
			"category_count", len(s.GetCategories()),
			"per_check_timeout_ms", s.GetPerCheckTimeoutMs(),
		)
	default:
		return slog.Group("request")
	}
}

// RPCResponseAttr is the response counterpart. Reports just the
// summary numbers an operator skims for ("how many entries did we
// list", "what was the exit code") rather than the full payload —
// large blobs (file contents, process tables) bloat the log without
// adding diagnostic value once you have the request fields.
func RPCResponseAttr(payload any) slog.Attr {
	switch r := payload.(type) {
	case *v2pb.RpcResponse_Exec:
		e := r.Exec
		return slog.Group("response",
			"exit_code", e.GetExitCode(),
			"stdout_bytes", len(e.GetStdout()),
			"stderr_bytes", len(e.GetStderr()),
		)
	case *v2pb.RpcResponse_ListDir:
		return slog.Group("response",
			"entry_count", len(r.ListDir.GetEntries()),
		)
	case *v2pb.RpcResponse_Stat:
		e := r.Stat.GetEntry()
		if e == nil {
			return slog.Group("response")
		}
		return slog.Group("response",
			"size_bytes", e.GetSize(),
			"mode_octal", fmt.Sprintf("%04o", e.GetMode()),
		)
	case *v2pb.RpcResponse_Delete, *v2pb.RpcResponse_Rename, *v2pb.RpcResponse_Mkdir, *v2pb.RpcResponse_Chmod:
		return slog.Group("response")
	case *v2pb.RpcResponse_ProcessList:
		p := r.ProcessList
		return slog.Group("response",
			"total_count", p.GetTotalCount(),
			"returned_count", len(p.GetProcesses()),
		)
	case *v2pb.RpcResponse_SysInfo:
		s := r.SysInfo
		return slog.Group("response",
			"hostname", s.GetHostname(),
			"platform", s.GetPlatform(),
			"primary_ip", s.GetPrimaryIp(),
		)
	case *v2pb.RpcResponse_SecurityScan:
		s := r.SecurityScan
		return slog.Group("response",
			"finding_count", len(s.GetFindings()),
			"check_count", len(s.GetChecks()),
			"elapsed_ms", s.GetElapsedMs(),
		)
	default:
		return slog.Group("response")
	}
}
