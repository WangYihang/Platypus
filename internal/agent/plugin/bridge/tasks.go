package bridge

import (
	"context"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

const tasksPluginID = "com.platypus.sys-tasks-windows"

// TasksList forwards to com.platypus.sys-tasks-windows's list_tasks
// RPC (PowerShell Get-ScheduledTask + Get-ScheduledTaskInfo).
// Sibling of CronList for the Windows side.
func TasksList(reg *plugin.Registry) func(ctx context.Context, req *v2pb.TasksListRequest) *v2pb.TasksListResponse {
	return func(ctx context.Context, req *v2pb.TasksListRequest) *v2pb.TasksListResponse {
		var out v2pb.TasksListResponse
		errStr, err := invokeProto(ctx, reg, tasksPluginID, "list_tasks", req, &out)
		if err != nil {
			return &v2pb.TasksListResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.TasksListResponse{Error: errStr}
		}
		return &out
	}
}
