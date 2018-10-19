package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) Command(args []string) {
	if context.Ctx.Current == nil {
		log.Error("Current session is not set, please use `Jump` command to set the interactive Command")
		return
	}

	command := strings.Join(args, " ")
	log.Info("Execute %s on %s", command, context.Ctx.Current.Desc())

	result := context.Ctx.Current.SystemToken(command)
	log.Info("Result: %s", result)
}

func (dispatcher Dispatcher) CommandHelp(args []string) {
	fmt.Println("Usage of Command")
	fmt.Println("\tCommand")
}

func (dispatcher Dispatcher) CommandDesc(args []string) {
	fmt.Println("Command")
	fmt.Println("\tPop up a interactive session, you can communicate with it via stdin/stdout")
}
