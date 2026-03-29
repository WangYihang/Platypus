package cmd

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/spf13/cobra"
)

var commandCmd = &cobra.Command{
	Use:   "Command [CMD...]",
	Short: "Execute a command on the current session",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
			log.Error("Current session is not set, please use `Jump` command to set it")
			return
		}
		command := strings.Join(args, " ")
		if core.Ctx.Current != nil {
			log.Info("Execute %s on %s", command, core.Ctx.Current.FullDesc())
			result := core.Ctx.Current.SystemToken(command)
			log.Info("Result: %s", result)
			return
		}
		if core.Ctx.CurrentTermite != nil {
			log.Info("Execute %s on %s", command, core.Ctx.CurrentTermite.FullDesc())
			result := core.Ctx.CurrentTermite.System(command)
			log.Info("Result: %s", result)
		}
	},
}
