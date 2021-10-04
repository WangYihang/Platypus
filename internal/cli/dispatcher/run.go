package dispatcher

import (
	"fmt"
	"strconv"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/log"
	"github.com/c-bata/go-prompt"
)

type Argument struct {
	Name        string
	Desc        string
	IsFlag      bool
	AllowRepeat bool
	IsRequired  bool
	Default     interface{}
	SuggestFunc func(name string) []prompt.Suggest
}

func (dispatcher CommandDispatcher) RunArgumentsSuggestion(name string) []prompt.Suggest {
	switch name {
	case "host":
		return []prompt.Suggest{{Text: "192.168.1.1", Description: "eth0"}}
	case "port":
		return []prompt.Suggest{}
	default:
		return []prompt.Suggest{}
	}
}

func (dispatcher CommandDispatcher) RunArguments() []Argument {
	arguments := []Argument{
		{Name: "host", Desc: "todo", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: dispatcher.RunArgumentsSuggestion},
		{Name: "port", Desc: "todo", IsFlag: false, IsRequired: true, AllowRepeat: false, Default: nil, SuggestFunc: dispatcher.RunArgumentsSuggestion},
		{Name: "debug", Desc: "todo", IsFlag: true, IsRequired: false, AllowRepeat: false, Default: false, SuggestFunc: dispatcher.RunArgumentsSuggestion},
	}
	return arguments
}

func (dispatcher CommandDispatcher) Run(args []string) {
	if len(args) != 2 {
		log.Error("Arguments error, use `Help Run` to get more information")
		dispatcher.RunHelp()
		return
	}

	host := args[0]
	port, err := strconv.ParseUint(args[1], 10, 16)
	if err != nil {
		log.Error("Invalid port: %s, use `Help Run` to get more information", args[1])
		dispatcher.RunHelp()
		return
	}

	server := context.CreateTCPServer(host, uint16(port), "", false, true, "")
	if server != nil {
		go (*server).Run()
	}
}

func (dispatcher CommandDispatcher) RunHelp() string {
	fmt.Println("Usage of Run")
	fmt.Println("\tRun [HOST] [PORT]")
	fmt.Println("\tHOST\tTHe host you want to listen on")
	fmt.Println("\tPORT\tTHe port you want to listen on")
	return ""
}

func (dispatcher CommandDispatcher) RunDesc() string {
	fmt.Println("Run")
	fmt.Println("\tTry to run a server, listening on a port, waiting for client to connect")
	return "Run a server"
}
