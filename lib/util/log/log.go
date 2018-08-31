package log

import (
	"fmt"
	"time"
	"github.com/fatih/color"
)

const (
	debug   = "[DEBUG]"
	info    = "[INFO]"
	err     = "[ERROR]"
	warn    = "[WARN]"
	success = "[SUCCESS]"
)

func Debug(format string, a ...interface{}) {
	color.New(color.FgYellow).Printf(debug + " ")
	color.New(color.FgHiBlack).Printf(formatTime() + " ")
	fmt.Println(fmt.Sprintf(format, a...))
}

func Info(format string, a ...interface{}) {
	color.New(color.FgBlue).Printf(info + " ")
	color.New(color.FgHiBlack).Printf(formatTime() + " ")
	fmt.Println(fmt.Sprintf(format, a...))
}

func Error(format string, a ...interface{}) {
	color.New(color.FgRed).Printf(err + " ")
	color.New(color.FgHiBlack).Printf(formatTime() + " ")
	fmt.Println(fmt.Sprintf(format, a...))
}
func Warn(format string, a ...interface{}) {
	color.New(color.FgMagenta).Printf(warn + " ")
	color.New(color.FgHiBlack).Printf(formatTime() + " ")
	fmt.Println(fmt.Sprintf(format, a...))
}

func Success(format string, a ...interface{}) {
	color.New(color.FgGreen).Printf(success + " ")
	color.New(color.FgHiBlack).Printf(formatTime() + " ")
	fmt.Println(fmt.Sprintf(format, a...))
}

func CommandPrompt(commandPrompt string) {
	color.New(color.FgYellow).Print(commandPrompt)
}

func formatTime() string {
	return time.Now().Format("2006/01/02 15:04:05")
}
