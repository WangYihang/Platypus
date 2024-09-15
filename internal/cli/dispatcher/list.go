package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) List(args []string) {
	if len(context.Ctx.Servers) == 0 {
		log.Warn("No listening servers")
		return
	}
	log.Info("Listing %d listening servers", len(context.Ctx.Servers))

	for _, server := range context.Ctx.Servers {
		server.AsTable()
	}
}

func (dispatcher commandDispatcher) ListHelp(args []string) {
	fmt.Println("Usage of List")
	fmt.Println("\tList")
}

func (dispatcher commandDispatcher) ListDesc(args []string) {
	fmt.Println("List")
	fmt.Println("\tTry list all listening servers and connected clients")
}
