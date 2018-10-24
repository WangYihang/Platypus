package log

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fatih/color"
)

var logger = log.New(os.Stderr, "", log.Ldate|log.Ltime)

const (
	debug   = "[DEBUG]"
	info    = "[INFO]"
	err     = "[ERROR]"
	warn    = "[WARN]"
	success = "[SUCCESS]"
	data    = "[DATA]"
	tunnel  = "[TUNNEL]"
)

var enabled = []string{
	info,
	// err,
	warn,
	// debug,
	success,
	// data,
	tunnel,
}

func printMessagePrefix(colorNumber color.Attribute, message string) {
	color.New(colorNumber).Printf(message + " ")
	color.New(color.FgHiBlack).Printf(formatTime() + " ")
}

func Tunnel(format string, a ...interface{}) {
	for _, mode := range enabled {
		if mode == "[TUNNEL]" {
			color.Set(color.FgCyan)
			logger.Print(fmt.Sprintf(format, a...))
			color.Unset()
			return
		}
	}
}

func Data(format string, a ...interface{}) {
	for _, mode := range enabled {
		if mode == "[DATA]" {
			color.Set(color.FgMagenta)
			logger.Print(fmt.Sprintf(format, a...))
			color.Unset()
			return
		}
	}
}

func Debug(format string, a ...interface{}) {
	for _, mode := range enabled {
		if mode == "[DEBUG]" {
			color.Set(color.FgYellow)
			logger.Print(fmt.Sprintf(format, a...))
			color.Unset()
			return
		}
	}
}

func Info(format string, a ...interface{}) {
	for _, mode := range enabled {
		if mode == "[INFO]" {
			color.Set(color.FgBlue)
			logger.Print(fmt.Sprintf(format, a...))
			color.Unset()
			return
		}
	}
}

func Error(format string, a ...interface{}) {
	for _, mode := range enabled {
		if mode == "[ERROR]" {
			color.Set(color.FgRed)
			logger.Print(fmt.Sprintf(format, a...))
			color.Unset()
			return
		}
	}
}
func Warn(format string, a ...interface{}) {
	for _, mode := range enabled {
		if mode == "[WARN]" {
			color.Set(color.FgMagenta)
			logger.Print(fmt.Sprintf(format, a...))
			color.Unset()
			return
		}
	}
}

func Success(format string, a ...interface{}) {
	for _, mode := range enabled {
		if mode == "[SUCCESS]" {
			color.Set(color.FgGreen)
			logger.Print(fmt.Sprintf(format, a...))
			color.Unset()
			return
		}
	}
}

func formatTime() string {
	return time.Now().Format("2006/01/02 15:04:05")
}
