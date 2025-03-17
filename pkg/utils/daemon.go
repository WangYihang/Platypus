package utils

import (
	"os"

	"github.com/sevlyar/go-daemon"
	"go.uber.org/zap"
)

// DaemonOptions contains configuration options for daemon mode
type DaemonOptions struct {
	WorkDir string   // Working directory for daemon process
	Umask   int      // File mode creation mask
	Args    []string // Command line arguments for daemon process
	PidFile string   // Optional PID file path
}

// DefaultDaemonOptions returns the default configuration for daemon mode
func DefaultDaemonOptions() *DaemonOptions {
	return &DaemonOptions{
		WorkDir: "/",
		Umask:   027,
		Args:    []string{},
	}
}

// StartDaemonMode starts the application in daemon mode.
// If successful, the parent process exits and the child continues as a daemon.
// The executable will be removed only in the daemon process.
// Returns nil if the process was successfully daemonized or exited as parent.
func StartDaemonMode(logger *zap.Logger, opts *DaemonOptions) error {
	if opts == nil {
		opts = DefaultDaemonOptions()
	}

	ctx := &daemon.Context{
		WorkDir: opts.WorkDir,
		Umask:   opts.Umask,
		Args:    opts.Args,
	}

	if opts.PidFile != "" {
		ctx.PidFileName = opts.PidFile
		ctx.PidFilePerm = 0644
	}

	child, err := ctx.Reborn()
	if err != nil {
		logger.Error("Failed to start daemon process", zap.Error(err))
		return err
	}

	// Parent process
	if child != nil {
		logger.Info("Parent process exiting, daemon started")
		os.Exit(0)
	}

	// Child (daemon) process
	logger.Info("Running as daemon process")
	defer ctx.Release()

	// Remove the executable that started this daemon
	if err := RemoveSelfExecutable(logger); err != nil {
		logger.Error("Failed to remove self executable", zap.Error(err))
	}

	return nil
}
