package ctx

import (
	"os"
	"path/filepath"

	"golang.org/x/term"
)

type Context struct {
	Token     string
	Host      string
	Port      uint16
	DistPort  uint16
	TermState *term.State
}

var Ctx Context

func IsValidToken(token string) bool {
	return token != ""
}

func SaveTermState() {
	oldState, err := term.GetState(int(os.Stdin.Fd()))
	if err != nil {
		return
	}
	Ctx.TermState = oldState
}

func RestoreTermState() {
	term.Restore(int(os.Stdin.Fd()), Ctx.TermState)
}

func GetHistoryFilepath() string {
	dirname, err := os.UserHomeDir()
	filename := ".platypus_history"
	if err != nil {
		return filename
	}
	return filepath.Join(dirname, filename)
}
