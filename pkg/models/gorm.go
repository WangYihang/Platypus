package models

import (
	"log/slog"
	"os"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	db, err := gorm.Open(sqlite.Open("db.sqlite3"), &gorm.Config{})
	if err != nil {
		slog.Error("failed to open database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	err = db.AutoMigrate(&LogEntry{})
	if err != nil {
		slog.Error("failed to migrate database", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
