package dispatcher

import (
	"fmt"
	"strconv"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/model/reverse"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) Run(args []string) {
	if len(args) != 3 {
		log.Error("Argments error, use `Help Run` to get more information")
		dispatcher.RunHelp([]string{})
		return
	}

	host := args[0]
	port, err := strconv.ParseInt(args[1], 10, 32)
	if err != nil {
		log.Error("Invalid port: %s, use `Help Run` to get more information", args[1])
		dispatcher.RunHelp([]string{})
		return
	}
	module := args[2]

	if module == "R" {
		server := reverse.CreateReverseTCPServer(host, int16(port))
		go server.Run()
		context.Ctx.AddServer(&server.TCPServer)
	} else if module == "C" {
		server := context.CreateTCPServer(host, int16(port))
		go server.Run()
		context.Ctx.AddServer(server)
	} else {
		log.Error("Invalid module: %s, use `Help Run` to get more information", args[1])
		dispatcher.RunHelp([]string{})
		return
	}
}

func (dispatcher Dispatcher) RunHelp(args []string) {
	fmt.Println("Usage of Run")
	fmt.Println("\tRun [HOST] [PORT] [MODELE]")
	fmt.Println("\tHOST\tTHe host you want to listen on")
	fmt.Println("\tPORT\tTHe port you want to listen on")
	fmt.Println("\tMODELE")
	fmt.Println("\t\tR\tReverse TCP Listener")
	fmt.Println("\t\tC\tCommon TCP Listener")
}

func (dispatcher Dispatcher) RunDesc(args []string) {
	fmt.Println("Run")
	fmt.Println("\tTry to run a server, listening on a port, waiting for client to connect")
}
