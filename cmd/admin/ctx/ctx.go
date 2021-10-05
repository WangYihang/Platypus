package ctx

import (
	"os"
	"path/filepath"
)

type Context struct {
	Token string
	Host  string
	Port  uint16
}

var Ctx Context

func IsValidToken(token string) bool {
	return token != ""
}

func GetHistoryFilepath() string {
	dirname, err := os.UserHomeDir()
	filename := ".platypus_history"
	if err != nil {
		return filename
	}
	return filepath.Join(dirname, filename)
}
