package cmd

import (
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "Info [HASH]",
	Short: "Display information about a node",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
				log.Error("Interactive session is not set, please use `Jump` to set it")
				return
			}
			if core.Ctx.Current != nil {
				core.Ctx.Current.AsTable()
				return
			}
			if core.Ctx.CurrentTermite != nil {
				core.Ctx.CurrentTermite.AsTable()
				return
			}
		}
		clue := args[0]
		if c := core.Ctx.FindTCPClientByHash(clue); c != nil {
			c.AsTable()
			return
		}
		if c := core.Ctx.FindTermiteClientByHash(clue); c != nil {
			c.AsTable()
			return
		}
		if s := core.Ctx.FindServerByHash(clue); s != nil {
			s.AsTable()
			return
		}
		log.Error("No such node")
	},
}
