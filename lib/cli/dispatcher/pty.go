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
	fmt.Println("\tFirst use `Jump` to select a client, then type `PTY`, then type `Interact` to drop into a fully interactive shell.")
	fmt.Println("\tYou can just simply type `exit` to exit pty mode")
}

func (dispatcher Dispatcher) PTYDesc(args []string) {
	fmt.Println("PTY")
	fmt.Println("\tTry to Spawn '/bin/bash' via Python, then the shell is fully interactive (You can use vim / htop and other stuffs)")
}
