package bridge

import (
	"context"
	"runtime"

	"github.com/WangYihang/Platypus/internal/agent/plugin"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// PkgPluginID returns the per-OS sys-pkg plugin id for the supplied
// runtime.GOOS value. Falls back to the linux plugin on unrecognised
// values — better to surface a "linux plugin doesn't exist on this
// host" error from the registry than silently no-op.
func PkgPluginID(goos string) string {
	switch goos {
	case "darwin":
		return "com.platypus.sys-pkg-darwin"
	case "windows":
		return "com.platypus.sys-pkg-windows"
	default:
		return "com.platypus.sys-pkg-linux"
	}
}

// PkgListInstalled forwards to the per-OS sys-pkg plugin's
// list_installed RPC. The returned response carries the detected
// backend ("apt", "brew", "winget", …) so the UI can render a
// per-host hint without an extra round trip.
func PkgListInstalled(reg *plugin.Registry) func(ctx context.Context, req *v2pb.PkgListInstalledRequest) *v2pb.PkgListInstalledResponse {
	return func(ctx context.Context, req *v2pb.PkgListInstalledRequest) *v2pb.PkgListInstalledResponse {
		var out v2pb.PkgListInstalledResponse
		errStr, err := invokeProto(ctx, reg, PkgPluginID(runtime.GOOS), "list_installed", req, &out)
		if err != nil {
			return &v2pb.PkgListInstalledResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.PkgListInstalledResponse{Error: errStr}
		}
		return &out
	}
}

// PkgListUpgradable forwards to list_upgradable on the same per-OS
// plugin. Returns the same backend hint in the response.
func PkgListUpgradable(reg *plugin.Registry) func(ctx context.Context, req *v2pb.PkgListUpgradableRequest) *v2pb.PkgListUpgradableResponse {
	return func(ctx context.Context, req *v2pb.PkgListUpgradableRequest) *v2pb.PkgListUpgradableResponse {
		var out v2pb.PkgListUpgradableResponse
		errStr, err := invokeProto(ctx, reg, PkgPluginID(runtime.GOOS), "list_upgradable", req, &out)
		if err != nil {
			return &v2pb.PkgListUpgradableResponse{Error: err.Error()}
		}
		if errStr != "" {
			return &v2pb.PkgListUpgradableResponse{Error: errStr}
		}
		return &out
	}
}
