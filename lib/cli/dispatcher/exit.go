package dispatcher

import (
	"os"

	"github.com/WangYihang/Platypus/lib/util/ui"
)

func (ctx Dispatcher) Exit(args []string) {
	if len(Servers) > 0 && !ui.PromptYesNo("There are listening servers, do you really want to exit?") {
		return
	}
	for _, server := range Servers {
		server.Stop()
		delete(Servers, server.Hash())
	}
	os.Exit(1)
}

func (ctx Dispatcher) ExitHelp(args []string) {

}

func (ctx Dispatcher) ExitDesc(args []string) {

}
