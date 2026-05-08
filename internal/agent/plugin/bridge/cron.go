package bridge

import (
	"context"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

const cronPluginID = "com.platypus.sys-cron-linux"

// CronList forwards to com.platypus.sys-cron-linux's list_cron_jobs
// RPC. Walks /etc/crontab, /etc/cron.d/*, /var/spool/cron/{,crontabs}/*,
// /etc/cron.{hourly,daily,weekly,monthly}/, and /etc/anacrontab.
//
// systemd timers are not in scope — covered by SystemdListUnits with
// the request's unit_type=timer.
func CronList(reg *plugin.Registry) func(ctx context.Context, req *v2pb.CronListRequest) *v2pb.CronListResponse {
	return func(ctx context.Context, req *v2pb.CronListRequest) *v2pb.CronListResponse {
		var out v2pb.CronListResponse
		errStr, err := invokeProto(ctx, reg, cronPluginID, "list_cron_jobs", req, &out)
		if err != nil {
			return &v2pb.CronListResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.CronListResponse{Error: errStr}
		}
		return &out
	}
}
