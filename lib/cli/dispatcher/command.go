package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) Command(args []string) {
	if len(args) == 0 {
		log.Error("Argments error, use `Help Command` to get more information")
		dispatcher.CommandHelp([]string{})
		return
	}

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
	fmt.Println("Usage of Command [CMD]")
	fmt.Println("\tCommand")
	fmt.Println("\tCMD\tThe command that you want to execute on the current session")
}

func (dispatcher Dispatcher) CommandDesc(args []string) {
	fmt.Println("Command")
	fmt.Println("\texecute a command on the current session")
}
