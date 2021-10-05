package connect

import (
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
)

type Command struct{}

func (command Command) Help() string {
	return "Connect"
}

func (command Command) Description() string {
	return "Connect"
}

func (command Command) Arguments() []meta.Argument {
	return []meta.Argument{}
}

func (command Command) Execute(args []string) {
	log.Info("Executing Connect: %v", args)
}

func (command Command) Suggest(name string) []prompt.Suggest {
	return []prompt.Suggest{}
}
