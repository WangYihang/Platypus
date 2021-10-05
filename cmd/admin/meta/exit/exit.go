package exit

import (
	"os"

	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/c-bata/go-prompt"
)

type Command struct{}

func (command Command) Name() string {
	return "Exit"
}

func (command Command) Help() string {
	return "Exit"
}

func (command Command) Description() string {
	return "Exit"
}

func (command Command) Arguments() []meta.Argument {
	return []meta.Argument{}
}

func (command Command) Execute(args []string) {
	ctx.RestoreTermState()
	os.Exit(0)
}

func (command Command) Suggest(name string, typed string) []prompt.Suggest {
	return []prompt.Suggest{}
}
