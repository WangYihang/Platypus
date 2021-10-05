package connect

import (
	"strconv"

	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
)

type Command struct{}

func (command Command) Name() string {
	return "Connect"
}

func (command Command) Help() string {
	return "Connect"
}

func (command Command) Description() string {
	return "Connect"
}

func (command Command) Arguments() []meta.Argument {
	return []meta.Argument{
		{Name: "host", Desc: "platypus restful api backend host", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
		{Name: "port", Desc: "platypus restful api backend port", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
	}
}

func (command Command) Execute(args []string) {
	result, err := meta.ParseArguments(command, args)
	if err != nil {
		log.Error(err.Error())
		return
	}
	host := *result["host"].(*string)
	port, err := strconv.Atoi(*result["port"].(*string))
	if err != nil {
		log.Error(err.Error())
		return
	}
	ctx.Ctx.Host = host
	ctx.Ctx.Port = uint16(port)
	log.Info("Setting endpoint to %s:%d", ctx.Ctx.Host, ctx.Ctx.Port)
}

func (command Command) Suggest(name string) []prompt.Suggest {
	return []prompt.Suggest{}
}
