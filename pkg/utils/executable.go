package utils

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

// RemoveSelfExecutable removes the self executable
func RemoveSelfExecutable(logger *zap.Logger) error {
	path, err := filepath.Abs(os.Args[0])
	if err != nil {
		logger.Error("failed to get absolute path", zap.String("error", err.Error()))
		return err
	}
	logger.Info("removing self executable", zap.String("path", path))
	err = os.Remove(path)
	if err != nil {
		logger.Error("failed to remove self executable", zap.String("path", path), zap.String("error", err.Error()))
		return err
	}
	logger.Info("self executable removed", zap.String("path", path))
	return nil
}
