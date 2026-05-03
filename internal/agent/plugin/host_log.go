package plugin

import (
	"context"

	extism "github.com/extism/go-sdk"

	"github.com/WangYihang/Platypus/internal/log"
)

// hostLog is implicitly granted to every plugin (CapLog is added in
// loader/install). The wasm side passes (level i32, msg_offset i64)
// and gets back an i32 status; level mapping lives in decodeLogLevel.
//
// Two sinks: the per-plugin in-memory ring (logSink, served back by
// PluginGetLogsResponse) and the host's structured log
// (log.L), so operators can correlate plugin output with the rest of
// the agent's activity stream by plugin_id + correlation_id.
func (pctx *pluginCtx) hostLog(_ context.Context, p *extism.CurrentPlugin, stack []uint64) {
	level := decodeLogLevel(uint32(stack[0]))
	msg, err := readStringArg(p, stack[1])
	if err != nil {
		stack[0] = 1
		return
	}
	corr := pctx.correlationID()
	pctx.logSink.append(pctx.now(), level, msg, corr)
	args := []any{"plugin_id", pctx.id, "correlation_id", corr, "msg", msg}
	switch level {
	case "debug":
		log.L.Debug("plugin.log", args...)
	case "warn":
		log.L.Warn("plugin.log", args...)
	case "error":
		log.L.Error("plugin.log", args...)
	default:
		log.L.Info("plugin.log", args...)
	}
	stack[0] = 0
}

// decodeLogLevel maps the wasm-side i32 to the same set of strings
// internal/log uses on the host side. Unknown values default to
// "info" so a plugin built against a future ABI doesn't spam the log
// with empty levels.
func decodeLogLevel(v uint32) string {
	switch v {
	case 0:
		return "debug"
	case 1:
		return "info"
	case 2:
		return "warn"
	case 3:
		return "error"
	default:
		return "info"
	}
}
