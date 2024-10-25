package models

import (
	"gorm.io/gorm"
)

// LogLevel represents the log level of a log entry
type LogLevel string

const (
	// LogLevelError represents an error log level
	LogLevelError LogLevel = "error"
	// LogLevelInfo represents an info log level
	LogLevelInfo LogLevel = "info"
	// LogLevelDebug represents a debug log level
	LogLevelDebug LogLevel = "debug"
)

// LogEntry represents a log entry in the database
type LogEntry struct {
	gorm.Model
	Level   LogLevel
	Message string
}
