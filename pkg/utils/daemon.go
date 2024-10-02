package utils

import (
	"os"

	"github.com/sevlyar/go-daemon"
	"go.uber.org/zap"
)

// StartDaemonMode starts the application in daemon mode
// The executable will be reborn as a daemon process and then removed
func StartDaemonMode(logger *zap.Logger) error {
	logger.Info("switching to daemon mode")
	ctx := &daemon.Context{
		WorkDir: "/",
		Umask:   027,
		Args:    []string{},
	}
	logger.Info("reborning process")
	d, err := ctx.Reborn()
	if err != nil {
		logger.Error("failed to reborn process", zap.String("error", err.Error()))
		return err
	}
	if d != nil {
		logger.Info("exiting parent process")
		os.Exit(0)
	}
	defer func() {
		err := ctx.Release()
		if err != nil {
			logger.Error("failed to release context", zap.String("error", err.Error()))
		}
	}()
	logger.Info("daemon process started")
	if err := RemoveSelfExecutable(logger); err != nil {
		logger.Warn("failed to remove self executable", zap.String("error", err.Error()))
	}
	return nil
}
