package dispatcher

import (
	"fmt"
	"strconv"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) Run(args []string) {
	if len(args) != 2 {
		log.Error("Arguments error, use `Help Run` to get more information")
		dispatcher.RunHelp([]string{})
		return
	}

	host := args[0]
	port, err := strconv.ParseUint(args[1], 10, 16)
	if err != nil {
		log.Error("Invalid port: %s, use `Help Run` to get more information", args[1])
		dispatcher.RunHelp([]string{})
		return
	}

	server := context.CreateTCPServer(host, uint16(port), "", false, true, "", "")
	if server != nil {
		go (*server).Run()
	}
}

func (dispatcher commandDispatcher) RunHelp(args []string) {
	fmt.Println("Usage of Run")
	fmt.Println("\tRun [HOST] [PORT]")
	fmt.Println("\tHOST\tTHe host you want to listen on")
	fmt.Println("\tPORT\tTHe port you want to listen on")
}

func (dispatcher commandDispatcher) RunDesc(args []string) {
	fmt.Println("Run")
	fmt.Println("\tTry to run a server, listening on a port, waiting for client to connect")
}
