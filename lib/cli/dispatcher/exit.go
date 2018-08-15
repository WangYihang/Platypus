package dispatcher

import (
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/ui"
)

func (ctx Dispatcher) Exit(args []string) {
	if len(context.Servers) > 0 && !ui.PromptYesNo("There are listening servers, do you really want to exit?") {
		return
	}
	for _, server := range context.Servers {
		server.Stop()
		delete(context.Servers, server.Hash)
	}
	os.Exit(1)
}

func (ctx Dispatcher) ExitHelp(args []string) {
	fmt.Println("Usage of Exit")
	fmt.Println("\tExit")
}

func (ctx Dispatcher) ExitDesc(args []string) {
	fmt.Println("Exit")
	fmt.Println("\tThis command will try to exit the whole process")
	fmt.Println("\tIf there is any listening server, it will ask you to stop them or not")
}
