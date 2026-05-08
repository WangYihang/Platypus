package bridge

import (
	"context"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

const fileCapsPluginID = "com.platypus.sys-file-caps-linux"

// FileCapsList forwards to com.platypus.sys-file-caps-linux's
// list_file_caps RPC. Companion to the SUID outliers check in
// sys-security: lists every binary with a Linux file capability set
// (xattr `security.capability`), classifies by risk, and tags the
// well-known cap'd binaries (ping, mtr, traceroute, …) so operators
// see outliers by default.
func FileCapsList(reg *plugin.Registry) func(ctx context.Context, req *v2pb.FileCapsListRequest) *v2pb.FileCapsListResponse {
	return func(ctx context.Context, req *v2pb.FileCapsListRequest) *v2pb.FileCapsListResponse {
		var out v2pb.FileCapsListResponse
		errStr, err := invokeProto(ctx, reg, fileCapsPluginID, "list_file_caps", req, &out)
		if err != nil {
			return &v2pb.FileCapsListResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.FileCapsListResponse{Error: errStr}
		}
		return &out
	}
}
