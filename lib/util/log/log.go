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

func printMessagePrefix(colorNumber color.Attribute) {
	color.New(colorNumber).Printf(debug + " ")
	color.New(color.FgHiBlack).Printf(formatTime() + " ")
}

func Debug(format string, a ...interface{}) {
	printMessagePrefix(color.FgYellow)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}

func Info(format string, a ...interface{}) {
	printMessagePrefix(color.FgBlue)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}

func Error(format string, a ...interface{}) {
	printMessagePrefix(color.FgRed)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}
func Warn(format string, a ...interface{}) {
	printMessagePrefix(color.FgMagenta)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}

func Success(format string, a ...interface{}) {
	printMessagePrefix(color.FgGreen)
	fmt.Fprintln(os.Stderr, fmt.Sprintf(format, a...))
}

func CommandPrompt(commandPrompt string) {
	color.New(color.FgYellow).Print(commandPrompt)
}

func formatTime() string {
	return time.Now().Format("2006/01/02 15:04:05")
}
