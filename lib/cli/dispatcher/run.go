package dispatcher

import (
	"fmt"
	"strconv"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/model"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) Run(args []string) {
	if len(args) != 2 {
		log.Error("Argments error, use `Help Run` to get more information")
		ctx.RunHelp([]string{})
		return
	}
	host := args[0]
	port, err := strconv.ParseInt(args[1], 10, 32)
	if err != nil {
		log.Error("Invalid port: %s, use `Help Run` to get more information", args[1])
		ctx.RunHelp([]string{})
		return
	}
	server := model.CreateServer(host, int16(port))
	listener, err := server.Listen()
	if err != nil {
		fmt.Println(err)
	}
	context.Ctx.Servers[server.Hash] = server
	go context.Ctx.RunServer(server, listener)
}

func (ctx Dispatcher) RunHelp(args []string) {
	fmt.Println("Usage of Run")
	fmt.Println("\tRun [HOST] [PORT]")
	fmt.Println("\tHOST\tTHe host you want to listen on")
	fmt.Println("\tPORT\tTHe port you want to listen on")
}

func (ctx Dispatcher) RunDesc(args []string) {
	fmt.Println("Run")
	fmt.Println("\tTry to run a server, listening on a port, waiting for client to connect")
}
