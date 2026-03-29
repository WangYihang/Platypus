package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) Command(args []string) {
	if len(args) == 0 {
		log.Error("Arguments error, use `Help Command` to get more information")
		dispatcher.CommandHelp([]string{})
		return
	}

	if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
		log.Error("Current session is not set, please use `Jump` command to set the interactive Command")
		return
	}

	if core.Ctx.Current != nil {
		command := strings.Join(args, " ")
		log.Info("Execute %s on %s", command, core.Ctx.Current.FullDesc())

		result := core.Ctx.Current.SystemToken(command)
		log.Info("Result: %s", result)
		return
	}

	if core.Ctx.CurrentTermite != nil {
		command := strings.Join(args, " ")
		log.Info("Execute %s on %s", command, core.Ctx.CurrentTermite.FullDesc())

		result := core.Ctx.CurrentTermite.System(command)
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
