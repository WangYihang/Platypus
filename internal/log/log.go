// Package log provides JSON-formatted structured logging built on
// log/slog, tee'd to stderr and ./platypus.log. It keeps the legacy
// format-string API (Info/Warn/Error/etc.) so existing call sites keep
// working while emitting JSON on the wire.
package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// L is the shared structured logger. Use it directly for new code that
// wants typed attrs (e.g. L.Info("listener_ready", "port", 13337)).
var L *slog.Logger

var (
	initOnce sync.Once
	logFile  *os.File
)

// logPath is where the JSON log file is written. It sits alongside the
// SQLite DB (./platypus.db) to keep operator files together.
const logPath = "./platypus.log"

// resolveLevel honours PLATYPUS_LOG_LEVEL for operators who need to
// peek at dispatcher / handshake debug lines (ingress_unknown_alpn,
// ingress_handshake_failed) without rebuilding. Unrecognised values
// fall back to INFO.
func resolveLevel() slog.Level {
	switch strings.ToLower(os.Getenv("PLATYPUS_LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func init() {
	// Default handler covers any log calls that happen before Init() —
	// currently none, but keeps things safe if an import-time hook logs.
	L = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: resolveLevel()}))
}

// Init opens ./platypus.log and switches the package logger to a JSON
// handler that fans out to stderr and the file. Safe to call more than
// once; only the first call does anything. If the file cannot be opened
// the logger stays on stderr-only and a warning is emitted.
func Init() {
	initOnce.Do(func() {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		var w io.Writer = os.Stderr
		level := resolveLevel()
		if err != nil {
			L = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
			L.Warn("log_file_open_failed", "path", logPath, "error", err.Error())
			return
		}
		logFile = f
		w = io.MultiWriter(os.Stderr, f)
		L = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
	})
}

// Close flushes and closes the log file if one is open. Callers don't
// generally need this — the OS reclaims the fd on exit — but it's useful
// for tests.
func Close() error {
	if logFile != nil {
		return logFile.Close()
	}
	return nil
}

// SetBaseFields permanently extends L with a fixed set of attributes
// emitted on every subsequent log line. Call exactly once at process
// startup with whatever always-on context the binary has —
// service, hostname, agent_id, version. Not safe to call concurrently
// with active log calls; the contract is "wire it in main() before
// goroutines spawn".
func SetBaseFields(attrs ...any) {
	L = L.With(attrs...)
}

func Data(format string, a ...interface{}) {
	L.Info(fmt.Sprintf(format, a...), "kind", "data")
}

func Debug(format string, a ...interface{}) {
	L.Debug(fmt.Sprintf(format, a...))
}

func Info(format string, a ...interface{}) {
	L.Info(fmt.Sprintf(format, a...))
}

func Error(format string, a ...interface{}) {
	L.Error(fmt.Sprintf(format, a...))
}

func Warn(format string, a ...interface{}) {
	L.Warn(fmt.Sprintf(format, a...))
}

func Success(format string, a ...interface{}) {
	L.Info(fmt.Sprintf(format, a...), "kind", "success")
}
