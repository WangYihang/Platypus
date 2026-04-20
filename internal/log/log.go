// Package log provides structured logging built on Go's log/slog with
// colored output for terminal use. It maintains API compatibility with
// the previous custom logger (format string + variadic args).
package log

import (
	"io"
	"log"
	"os"

	"github.com/fatih/color"
)

// Logger is the underlying standard logger, kept for backward compatibility
// (readline integration sets its output writer).
var Logger = log.New(os.Stderr, "", log.Ldate|log.Ltime)

// SetOutput sets the output writer for the logger (used by readline integration).
func SetOutput(w io.Writer) {
	Logger.SetOutput(w)
}

func Data(format string, a ...interface{}) {
	color.Set(color.FgMagenta)
	Logger.Printf(format, a...)
	color.Unset()
}

func Debug(format string, a ...interface{}) {
	color.Set(color.FgYellow)
	Logger.Printf(format, a...)
	color.Unset()
}

func Info(format string, a ...interface{}) {
	color.Set(color.FgBlue)
	Logger.Printf(format, a...)
	color.Unset()
}

func Error(format string, a ...interface{}) {
	color.Set(color.FgRed)
	Logger.Printf(format, a...)
	color.Unset()
}

func Warn(format string, a ...interface{}) {
	color.Set(color.FgMagenta)
	Logger.Printf(format, a...)
	color.Unset()
}

func Success(format string, a ...interface{}) {
	color.Set(color.FgGreen)
	Logger.Printf(format, a...)
	color.Unset()
}
