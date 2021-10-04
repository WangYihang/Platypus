package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/context"
)

func (dispatcher CommandDispatcher) Exit(args []string) {
	context.Shutdown()
}

func (dispatcher CommandDispatcher) ExitHelp() string {
	fmt.Println("Usage of Exit")
	fmt.Println("\tExit")
	return ""
}

func (dispatcher CommandDispatcher) ExitDesc() string {
	fmt.Println("Exit")
	fmt.Println("\tExit the whole process")
	fmt.Println("\tIf there is any listening server, it will ask you to stop them or not")
	return ""
}
