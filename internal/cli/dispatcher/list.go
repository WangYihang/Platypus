package dispatcher

import (
	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/log"
)

func (dispatcher CommandDispatcher) List(args []string) {
	if len(context.Ctx.Servers) == 0 {
		log.Warn("No listening servers")
		return
	}
	log.Info("Listing %d listening servers", len(context.Ctx.Servers))

	for _, server := range context.Ctx.Servers {
		server.AsTable()
	}
}

func (dispatcher CommandDispatcher) ListHelp() string {
	// fmt.Println("Usage of List")
	// fmt.Println("\tList")
	return ""
}

func (dispatcher CommandDispatcher) ListDesc() string {
	// fmt.Println("List")
	// fmt.Println("\tTry list all listening servers and connected clients")
	return ""
}
