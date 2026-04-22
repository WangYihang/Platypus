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

func init() {
	// Default handler covers any log calls that happen before Init() —
	// currently none, but keeps things safe if an import-time hook logs.
	L = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// Init opens ./platypus.log and switches the package logger to a JSON
// handler that fans out to stderr and the file. Safe to call more than
// once; only the first call does anything. If the file cannot be opened
// the logger stays on stderr-only and a warning is emitted.
func Init() {
	initOnce.Do(func() {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		var w io.Writer = os.Stderr
		if err != nil {
			L = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
			L.Warn("log_file_open_failed", "path", logPath, "error", err.Error())
			return
		}
		logFile = f
		w = io.MultiWriter(os.Stderr, f)
		L = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
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
