package compile

import (
	"fmt"
	"strconv"

	server_api "github.com/WangYihang/Platypus/cmd/admin/api/server"
	"github.com/WangYihang/Platypus/cmd/admin/ctx"
	"github.com/WangYihang/Platypus/cmd/admin/meta"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/WangYihang/Platypus/internal/util/network"
	"github.com/c-bata/go-prompt"
)

type Command struct{}

func (command Command) Name() string {
	return "Compile"
}

func (command Command) Help() string {
	return "Compile"
}

func (command Command) Description() string {
	return "Compile"
}

func (command Command) Arguments() []meta.Argument {
	return []meta.Argument{
		{Name: "host", Desc: "platypus termite listening host", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
		{Name: "port", Desc: "platypus termite listening port", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
		{Name: "os", Desc: "platypus termite binary os", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
		{Name: "arch", Desc: "platypus termite binary arch", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
		{Name: "save", Desc: "save binary", IsFlag: false, IsRequired: false, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
		{Name: "upx", Desc: "compress with upx", IsFlag: false, IsRequired: false, AllowRepeat: false, Default: nil, SuggestFunc: command.Suggest},
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
	host := *result["host"].(*string)
	port, err := strconv.Atoi(*result["port"].(*string))
	if err != nil {
		log.Error(err.Error())
		return
	}
	os := *result["os"].(*string)
	arch := *result["arch"].(*string)
	save := *result["save"].(*string)
	upx, err := strconv.Atoi(*result["upx"].(*string))
	if err != nil {
		log.Warn("Invalid upx level: %s, upx compression is disabled", *result["upx"].(*string))
		upx = 0
	}

	// Do compile
	log.Info("%s:%d-%s-%s compiling...", host, port, os, arch)
	filename, err := server_api.Compile(host, uint16(port), os, arch, upx)

	if err != nil {
		log.Error(err.Error())
		return
	}

	// Get distribuotr port
	distPort := server_api.GetDistribuorPort()

	// Build downloading url
	url := fmt.Sprintf("http://%s:%d/%s", ctx.Ctx.Host, distPort, filename)
	log.Success("%s compiled, click (%s) to download.", filename, url)

	if save != "" {
		log.Info("Downloading %s into %s", url, save)
		err := network.DownloadFile(url, save, 755)
		if err != nil {
			log.Error(err.Error())
		}
	}
}

func (command Command) Suggest(name string, typed string) []prompt.Suggest {
	if !ctx.IsValidToken(ctx.Ctx.Token) {
		return []prompt.Suggest{}
	}
	switch name {
	case "host":
		suggests := []prompt.Suggest{}
		for _, server := range server_api.GetServers().Servers {
			if server.Encrypted {
				for _, host := range server.Interfaces {
					suggest := prompt.Suggest{
						Text:        host,
						Description: server.OnelineDesc(),
					}
					suggests = append(suggests, suggest)
				}
				suggest := prompt.Suggest{
					Text:        server.PublicIP,
					Description: server.OnelineDesc(),
				}
				suggests = append(suggests, suggest)
			}
		}
		return suggests
	case "port":
		suggests := []prompt.Suggest{}
		for _, server := range server_api.GetServers().Servers {
			if server.Encrypted {
				suggest := prompt.Suggest{
					Text:        fmt.Sprintf("%d", server.Port),
					Description: server.OnelineDesc(),
				}
				suggests = append(suggests, suggest)
			}
		}
		return suggests
	case "os":
		return []prompt.Suggest{
			{Text: "linux", Description: "for Linux"},
			{Text: "darwin", Description: "for MacOS"},
			{Text: "windows", Description: "for Windows"},
		}
	case "arch":
		return []prompt.Suggest{
			{Text: "amd64", Description: ""},
			{Text: "386", Description: ""},
			{Text: "arm", Description: ""},
			{Text: "arm64", Description: ""},
		}
	case "save":
		return []prompt.Suggest{}
	case "upx":
		return []prompt.Suggest{
			{Text: "0", Description: "Disable upx compression"},
			{Text: "9", Description: "level 9"},
			{Text: "8", Description: "level 8 (default)"},
			{Text: "7", Description: "level 7"},
			{Text: "6", Description: "level 6"},
			{Text: "5", Description: "level 5"},
			{Text: "4", Description: "level 4"},
			{Text: "3", Description: "level 3"},
			{Text: "2", Description: "level 2"},
			{Text: "1", Description: "level 1"},
		}
	default:
		return []prompt.Suggest{}
	}
}
