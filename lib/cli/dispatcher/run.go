package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/session"
)

func (ctx Dispatcher) Run(args []string) {
	server := session.CreateServer("0.0.0.0", 4444)
	listener, err := server.Listen()
	if err != nil {
		fmt.Println(err)
	}
	Servers[server.Hash] = server
	go server.Run(listener)
}

func (ctx Dispatcher) RunHelp(args []string) {
	fmt.Println("Usage of Run")
	fmt.Println("\tRun [HOST] [PORT]")
	fmt.Println("\tHOST\tTHe host you want to listen on")
	fmt.Println("\tPORT\tTHe port you want to listen on")
}

func (ctx Dispatcher) RunDesc(args []string) {
	fmt.Println("Run")
	fmt.Println("\tThis command will try to run a server, listening on a port, waiting for client to connect")
}
