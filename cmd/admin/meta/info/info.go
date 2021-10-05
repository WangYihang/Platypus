package info

import (
	"fmt"

	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	server_controller "github.com/WangYihang/Platypus/internal/controller/server"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
	"github.com/imroc/req"
)

type Command struct{}

type Response struct {
	Status bool `json:"status"`
}

type ServersResponse struct {
	Response
	server_controller.ServersWithDistributorAddress `json:"msg"`
}

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

func GetServers() ServersResponse {
	authedHeader := req.Header{
		"Accept":        "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", ctx.Ctx.Token),
	}
	r, _ := req.Get("http://127.0.0.1:7331/api/v1/servers", authedHeader)
	sr := ServersResponse{}
	r.ToJSON(&sr)
	return sr
}

func (command Command) Execute(args []string) {
	log.Info("Executing Info: %v", args)
}

func (command Command) Suggest(name string) []prompt.Suggest {
	if ctx.IsValidToken(ctx.Ctx.Token) {
		suggests := []prompt.Suggest{}
		for _, server := range GetServers().Servers {
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
	} else {
		return []prompt.Suggest{}
	}
}
