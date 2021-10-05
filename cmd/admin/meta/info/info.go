package info

import (
	"fmt"

	server_api "github.com/WangYihang/Platypus/cmd/admin/api/server"
	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
)

type Command struct{}

func (command Command) Name() string {
	return "Info"
}

func (command Command) Help() string {
	return "Info"
}

func (command Command) Description() string {
	return "Info"
}

func (command Command) Arguments() []meta.Argument {
	return []meta.Argument{
		{Name: "hash", Desc: "hash of a client / server", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
	}
}

func (command Command) Execute(args []string) {
	if !ctx.IsValidToken(ctx.Ctx.Token) {
		log.Error("Invalid token: %s", ctx.Ctx.Token)
		return
	}
	result, err := meta.ParseArguments(command, args)
	if err != nil {
		log.Error(err.Error())
		return
	}
	hash := *result["hash"].(*string)
	log.Info("TODO: print info of: %s", hash)
}

func (command Command) Suggest(name string) []prompt.Suggest {
	if !ctx.IsValidToken(ctx.Ctx.Token) {
		return []prompt.Suggest{}
	}
	suggests := []prompt.Suggest{}
	for _, server := range server_api.GetServers().Servers {
		var description string
		if server.Encrypted {
			description = fmt.Sprintf("%s:%d, %d clients (encrypted)", server.Host, server.Port, len(server.TermiteClients))
		} else {
			description = fmt.Sprintf("%s:%d, %d clients (plain)", server.Host, server.Port, len(server.Clients))
		}
		suggest := prompt.Suggest{
			Text:        server.Hash,
			Description: description,
		}
		suggests = append(suggests, suggest)
	}
	return suggests

}
