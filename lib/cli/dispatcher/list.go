package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) List(args []string) {
	if len(context.Ctx.Servers) == 0 {
		log.Warn(fmt.Sprintf("No listening servers"))
		return
	}
	log.Info(fmt.Sprintf("Listing %d listening servers", len(context.Ctx.Servers)))

	for _, server := range context.Ctx.Servers {
		server.AsTable()
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
