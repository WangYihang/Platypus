package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/context"
)

func (dispatcher commandDispatcher) Exit(args []string) {
	context.Shutdown()
}

func (dispatcher commandDispatcher) ExitHelp(args []string) {
	fmt.Println("Usage of Exit")
	fmt.Println("\tExit")
}

func (dispatcher commandDispatcher) ExitDesc(args []string) {
	fmt.Println("Exit")
	fmt.Println("\tExit the whole process")
	fmt.Println("\tIf there is any listening server, it will ask you to stop them or not")
}
