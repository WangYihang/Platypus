package auth

import (
	"github.com/WangYihang/Platypus/internal/cmd"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
)

type Command struct{}

func (command Command) Help() string {
	return "Auth"
}

func (command Command) Description() string {
	return "Auth"
}

func (command Command) Arguments() []cmd.Argument {
	return []cmd.Argument{}
}

func (command Command) Execute(args []string) {
	log.Info("Executing Auth: %v", args)
}

func (command Command) Suggest(name string) []prompt.Suggest {
	return []prompt.Suggest{}
}
