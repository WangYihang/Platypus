package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) PTY(args []string) {
	if context.Ctx.Current == nil {
		log.Error("The current client is not set, please use `Jump` to set the current client")
		return
	}
	if err := context.Ctx.Current.EstablishPTY(); err != nil {
		log.Error("Establish PTY failed: %s", err)
	}
}

func (dispatcher Dispatcher) PTYHelp(args []string) {
	fmt.Println("Usage of PTY")
	fmt.Println("\tPTY [HOST] [PORT]")
	fmt.Println("\tHOST\tTHe host you want to listen on")
	fmt.Println("\tPORT\tTHe port you want to listen on")
}

func (dispatcher Dispatcher) PTYDesc(args []string) {
	fmt.Println("PTY")
	fmt.Println("\tTry to PTY a server, listening on a port, waiting for client to connect")
}
