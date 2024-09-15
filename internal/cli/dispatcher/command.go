package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) Command(args []string) {
	if len(args) == 0 {
		log.Error("Arguments error, use `Help Command` to get more information")
		dispatcher.CommandHelp([]string{})
		return
	}

	if context.Ctx.Current == nil && context.Ctx.CurrentTermite == nil {
		log.Error("Current session is not set, please use `Jump` command to set the interactive Command")
		return
	}

	if context.Ctx.Current != nil {
		command := strings.Join(args, " ")
		log.Info("Execute %s on %s", command, context.Ctx.Current.FullDesc())

		result := context.Ctx.Current.SystemToken(command)
		log.Info("Result: %s", result)
		return
	}

	if context.Ctx.CurrentTermite != nil {
		command := strings.Join(args, " ")
		log.Info("Execute %s on %s", command, context.Ctx.CurrentTermite.FullDesc())

		result := context.Ctx.CurrentTermite.System(command)
		log.Info("Result: %s", result)
		return
	}
}

func (dispatcher commandDispatcher) CommandHelp(args []string) {
	fmt.Println("Usage of Command [CMD]")
	fmt.Println("\tCommand")
	fmt.Println("\tCMD\tThe command that you want to execute on the current session")
}

func (dispatcher commandDispatcher) CommandDesc(args []string) {
	fmt.Println("Command")
	fmt.Println("\texecute a command on the current session")
}
