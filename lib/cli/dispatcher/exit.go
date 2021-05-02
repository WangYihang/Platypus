package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/context"
)

func (dispatcher Dispatcher) Exit(args []string) {
	context.Shutdown()
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
