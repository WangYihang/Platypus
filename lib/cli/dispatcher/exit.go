package dispatcher

import (
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/ui"
)

func (dispatcher Dispatcher) Exit(args []string) {
	if len(context.Ctx.Servers) > 0 && !ui.PromptYesNo("There are listening servers, do you really want to exit?") {
		return
	}
	for _, server := range context.Ctx.Servers {
		(*server).Stop()
		delete(context.Ctx.Servers, (*server).Hash())
	}
	os.Exit(0)
}

func (dispatcher Dispatcher) ExitHelp(args []string) {
	fmt.Println("Usage of Exit")
	fmt.Println("\tExit")
}

func (dispatcher Dispatcher) ExitDesc(args []string) {
	fmt.Println("Exit")
	fmt.Println("\tExit the whole process")
	fmt.Println("\tIf there is any listening server, it will ask you to stop them or not")
}
