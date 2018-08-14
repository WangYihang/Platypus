package log

import (
	"fmt"
	"time"
)

const (
	colorRed = uint8(iota + 91)
	colorGreen
	colorYellow
	colorBlue
	colorMagenta

	debug   = "[TRAC]"
	info    = "[INFO]"
	err     = "[ERROR]"
	warn    = "[WARN]"
	success = "[SUCCESS]"
)

func Debug(format string, a ...interface{}) {
	prefix := yellow(debug)
	fmt.Println(formatLog(prefix), fmt.Sprintf(format, a...))
}

func Info(format string, a ...interface{}) {
	prefix := blue(info)
	fmt.Println(formatLog(prefix), fmt.Sprintf(format, a...))
}

func Error(format string, a ...interface{}) {
	prefix := red(err)
	fmt.Println(formatLog(prefix), fmt.Sprintf(format, a...))
}
func Warn(format string, a ...interface{}) {
	prefix := magenta(warn)
	fmt.Println(formatLog(prefix), fmt.Sprintf(format, a...))
}

func Success(format string, a ...interface{}) {
	prefix := green(success)
	fmt.Println(formatLog(prefix), fmt.Sprintf(format, a...))
}

func CommandPrompt(commandPrompt string) {
	fmt.Print(blue(commandPrompt))
}

func red(s string) string {
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", colorRed, s)
}

func green(s string) string {
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", colorGreen, s)
}

func yellow(s string) string {
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", colorYellow, s)
}

func blue(s string) string {
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", colorBlue, s)
}

func magenta(s string) string {
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", colorMagenta, s)
}

func formatLog(prefix string) string {
	return time.Now().Format("2006/01/02 15:04:05") + " " + prefix + " "
}
