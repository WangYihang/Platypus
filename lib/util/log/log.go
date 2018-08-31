package log

import (
	"fmt"
	"os"
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

func printMessagePrefix(colorNumber color.Attribute, message string) {
	color.New(colorNumber).Printf(message + " ")
	
	color.New(color.FgHiBlack).Printf(formatTime() + " ")
}

func Debug(format string, a ...interface{}) {
	printMessagePrefix(color.FgYellow, debug)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}

func Info(format string, a ...interface{}) {
	printMessagePrefix(color.FgBlue, info)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}

func Error(format string, a ...interface{}) {
	printMessagePrefix(color.FgRed, err)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}
func Warn(format string, a ...interface{}) {
	printMessagePrefix(color.FgMagenta, warn)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}

func Success(format string, a ...interface{}) {
	printMessagePrefix(color.FgGreen, success)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}

func CommandPrompt(commandPrompt string) {
	color.New(color.FgYellow).Print(commandPrompt)
}

func formatTime() string {
	return time.Now().Format("2006/01/02 15:04:05")
}
