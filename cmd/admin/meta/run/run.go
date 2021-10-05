package run

import (
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
)

type Command struct{}

func (command Command) Help() string {
	return "Run"
}

func (command Command) Description() string {
	return "Run"
}

func (command Command) Arguments() []meta.Argument {
	return []meta.Argument{
		{Name: "host", Desc: "network interface to bind", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
		{Name: "port", Desc: "port to bind", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
		{Name: "termite", Desc: "enable encryption by termite", IsFlag: true, IsRequired: false, AllowRepeat: false, Default: false, SuggestFunc: nil},
		{Name: "debug", Desc: "enable debug", IsFlag: true, IsRequired: false, AllowRepeat: false, Default: false, SuggestFunc: nil},
		{Name: "help", Desc: "print help information", IsFlag: true, IsRequired: false, AllowRepeat: false, Default: false, SuggestFunc: nil},
	}
}

func (command Command) Execute(args []string) {
	log.Info("Executing Run: %v", args)
}

func (command Command) Suggest(name string) []prompt.Suggest {
	switch name {
	case "host":
		return []prompt.Suggest{{Text: "192.168.1.1", Description: "eth0"}}
	case "port":
		return []prompt.Suggest{}
	default:
		return []prompt.Suggest{}
	}
}
