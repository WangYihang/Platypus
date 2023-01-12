package count

import (
	"fmt"

	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
	"github.com/imroc/req/v3"
)

type Command struct{}

func (command Command) Name() string {
	return "Count"
}

func (command Command) Help() string {
	return "Count"
}

func (command Command) Description() string {
	return "Count"
}

func (command Command) Arguments() []meta.Argument {
	return []meta.Argument{
		{Name: "cpu", Desc: "Get CPU info", IsFlag: true, IsRequired: false, AllowRepeat: false, Default: true, SuggestFunc: command.Suggest},
		{Name: "gc", Desc: "Get GC info", IsFlag: true, IsRequired: false, AllowRepeat: false, Default: true, SuggestFunc: command.Suggest},
		{Name: "memory", Desc: "Get Memory info", IsFlag: true, IsRequired: false, AllowRepeat: false, Default: true, SuggestFunc: command.Suggest},
		{Name: "version", Desc: "Get Version info", IsFlag: true, IsRequired: false, AllowRepeat: false, Default: true, SuggestFunc: command.Suggest},
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

	client := req.C()

	for _, cmd := range command.Arguments() {
		if *result[cmd.Name].(*bool) {
			url := fmt.Sprintf("http://%s:%d/api/v1/runtime/{name}", ctx.Ctx.Host, ctx.Ctx.Port)
			response := client.Get(url). // Create a GET request with specified URL.
							SetHeader("Accept", "application/json").
							SetHeader("Authorization", fmt.Sprintf("Bearer %s", ctx.Ctx.Token)).
							SetPathParam("name", cmd.Name).
							SetResult(&result).
							EnableDump().
							Do()
			fmt.Println(response)
		}
	}
}

func (command Command) Suggest(name string, typed string) []prompt.Suggest {
	return []prompt.Suggest{}
}
