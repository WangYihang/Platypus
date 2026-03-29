package cmd

import (
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var gatherCmd = &cobra.Command{
	Use:   "Gather [HASH]",
	Short: "Gather client information from a session",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
				log.Error("Interactive session is not set, please use `Jump` to set it")
				return
			}
			if core.Ctx.Current != nil {
				core.Ctx.Current.GatherClientInfo(core.Ctx.Current.GetHashFormat())
				readLineInstance.SetPrompt(color.CyanString(core.Ctx.Current.GetPrompt()))
				return
			}
			if core.Ctx.CurrentTermite != nil {
				core.Ctx.CurrentTermite.GatherClientInfo(core.Ctx.CurrentTermite.GetHashFormat())
				readLineInstance.SetPrompt(color.CyanString(core.Ctx.CurrentTermite.GetPrompt()))
				return
			}
		}
		clue := args[0]
		if c := core.Ctx.FindTCPClientByHash(clue); c != nil {
			c.GatherClientInfo(c.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(c.GetPrompt()))
			return
		}
		if c := core.Ctx.FindTermiteClientByHash(clue); c != nil {
			c.GatherClientInfo(c.GetHashFormat())
			readLineInstance.SetPrompt(color.CyanString(c.GetPrompt()))
			return
		}
		if s := core.Ctx.FindServerByHash(clue); s != nil {
			for _, client := range s.Clients {
				client.GatherClientInfo(client.GetHashFormat())
			}
			return
		}
		log.Error("No such node")
	},
}
