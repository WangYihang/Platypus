package log

import (
	"fmt"
	"time"
)

func Info(message string) {
	fmt.Println(fmt.Sprintf("[%s][INFO] %s", time.Now().Format("2006-01-02 15:04:05"), message))
}

func Error(message string) {
	fmt.Println(fmt.Sprintf("[%s][ERROR] %s", time.Now().Format("2006-01-02 15:04:05"), message))
}

func Debug(message string) {
	fmt.Println(fmt.Sprintf("[%s][DEBUG] %s", time.Now().Format("2006-01-02 15:04:05"), message))
}

func Warn(message string) {
	fmt.Println(fmt.Sprintf("[%s][WARN] %s", time.Now().Format("2006-01-02 15:04:05"), message))
}

func Fatal(message string) {
	fmt.Println(fmt.Sprintf("[%s][FATAL] %s", time.Now().Format("2006-01-02 15:04:05"), message))
}

func Panic(message string) {
	fmt.Println(fmt.Sprintf("[%s][PANIC] %s", time.Now().Format("2006-01-02 15:04:05"), message))
}
