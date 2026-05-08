package bridge

import (
	"context"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// services.go covers the three OS-specific service-management
// plugins: sys-systemd-linux (systemctl), sys-services-darwin
// (launchctl), sys-services-windows (Get-Service). Their wire
// shapes diverge on list_units (different identifiers per platform)
// but converge on unit_action (a uniform "name + action → ok/exit").
//
// The three list_units functions are kept separate rather than
// hidden behind runtime.GOOS dispatch — operators can target a
// specific OS-of-fleet via the bulk RPC endpoint and the typed
// shape carries the per-platform fields (load/active/sub for
// systemd, label/pid for launchd, display_name/start_type for
// Windows) instead of a lossy union.

const (
	systemdPluginID  = "com.platypus.sys-systemd-linux"
	launchdPluginID  = "com.platypus.sys-services-darwin"
	winSvcPluginID   = "com.platypus.sys-services-windows"
)

// SystemdListUnits forwards to com.platypus.sys-systemd-linux's
// list_units RPC.
func SystemdListUnits(reg *plugin.Registry) func(ctx context.Context, req *v2pb.SystemdListUnitsRequest) *v2pb.SystemdListUnitsResponse {
	return func(ctx context.Context, req *v2pb.SystemdListUnitsRequest) *v2pb.SystemdListUnitsResponse {
		var out v2pb.SystemdListUnitsResponse
		errStr, err := invokeProto(ctx, reg, systemdPluginID, "list_units", req, &out)
		if err != nil {
			return &v2pb.SystemdListUnitsResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.SystemdListUnitsResponse{Error: errStr}
		}
		return &out
	}
}

// SystemdShowUnit forwards `systemctl show <unit>` and returns the
// flat property map.
func SystemdShowUnit(reg *plugin.Registry) func(ctx context.Context, req *v2pb.SystemdShowUnitRequest) *v2pb.SystemdShowUnitResponse {
	return func(ctx context.Context, req *v2pb.SystemdShowUnitRequest) *v2pb.SystemdShowUnitResponse {
		var out v2pb.SystemdShowUnitResponse
		errStr, err := invokeProto(ctx, reg, systemdPluginID, "show_unit", req, &out)
		if err != nil {
			return &v2pb.SystemdShowUnitResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.SystemdShowUnitResponse{Error: errStr}
		}
		return &out
	}
}

// SystemdUnitAction forwards a unit lifecycle action (start, stop,
// restart, reload, enable, disable, is-active, …) to systemctl.
func SystemdUnitAction(reg *plugin.Registry) func(ctx context.Context, req *v2pb.UnitActionRequest) *v2pb.UnitActionResponse {
	return func(ctx context.Context, req *v2pb.UnitActionRequest) *v2pb.UnitActionResponse {
		return unitActionInvoke(ctx, reg, systemdPluginID, req)
	}
}

// LaunchdListUnits forwards to com.platypus.sys-services-darwin's
// list_units RPC.
func LaunchdListUnits(reg *plugin.Registry) func(ctx context.Context, req *v2pb.LaunchdListUnitsRequest) *v2pb.LaunchdListUnitsResponse {
	return func(ctx context.Context, req *v2pb.LaunchdListUnitsRequest) *v2pb.LaunchdListUnitsResponse {
		var out v2pb.LaunchdListUnitsResponse
		errStr, err := invokeProto(ctx, reg, launchdPluginID, "list_units", req, &out)
		if err != nil {
			return &v2pb.LaunchdListUnitsResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.LaunchdListUnitsResponse{Error: errStr}
		}
		return &out
	}
}

// LaunchdUnitAction forwards a launchctl action (start, stop,
// kickstart, enable, disable, load, unload).
func LaunchdUnitAction(reg *plugin.Registry) func(ctx context.Context, req *v2pb.UnitActionRequest) *v2pb.UnitActionResponse {
	return func(ctx context.Context, req *v2pb.UnitActionRequest) *v2pb.UnitActionResponse {
		return unitActionInvoke(ctx, reg, launchdPluginID, req)
	}
}

// WindowsListServices forwards to com.platypus.sys-services-windows's
// list_units RPC (Get-Service).
func WindowsListServices(reg *plugin.Registry) func(ctx context.Context, req *v2pb.WindowsListServicesRequest) *v2pb.WindowsListServicesResponse {
	return func(ctx context.Context, req *v2pb.WindowsListServicesRequest) *v2pb.WindowsListServicesResponse {
		var out v2pb.WindowsListServicesResponse
		errStr, err := invokeProto(ctx, reg, winSvcPluginID, "list_units", req, &out)
		if err != nil {
			return &v2pb.WindowsListServicesResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.WindowsListServicesResponse{Error: errStr}
		}
		return &out
	}
}

// WindowsServiceAction forwards a Get-Service action (start, stop,
// restart, pause, continue).
func WindowsServiceAction(reg *plugin.Registry) func(ctx context.Context, req *v2pb.UnitActionRequest) *v2pb.UnitActionResponse {
	return func(ctx context.Context, req *v2pb.UnitActionRequest) *v2pb.UnitActionResponse {
		return unitActionInvoke(ctx, reg, winSvcPluginID, req)
	}
}

func unitActionInvoke(ctx context.Context, reg *plugin.Registry, pluginID string, req *v2pb.UnitActionRequest) *v2pb.UnitActionResponse {
	var out v2pb.UnitActionResponse
	errStr, err := invokeProto(ctx, reg, pluginID, "unit_action", req, &out)
	if err != nil {
		return &v2pb.UnitActionResponse{Error: err.Error()}
	}
	if errStr != "" {
		return &v2pb.UnitActionResponse{Error: errStr}
	}
	return &out
}
