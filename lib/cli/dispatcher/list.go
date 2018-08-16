package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) List(args []string) {
	log.Info(fmt.Sprintf("Listing %d servers", len(context.Ctx.Servers)))
	for _, server := range context.Ctx.Servers {
		fmt.Println(server.FullDesc())
	}
}

func (dispatcher Dispatcher) ListHelp(args []string) {
	fmt.Println("Usage of List")
	fmt.Println("\tList")
}

func (dispatcher Dispatcher) ListDesc(args []string) {
	fmt.Println("List")
	fmt.Println("\tTry list all listening servers and connected clients")
}
