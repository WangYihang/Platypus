package cmd

import (
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var jumpCmd = &cobra.Command{
	Use:   "Jump [HASH|ALIAS]",
	Short: "Jump to a session for interaction",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		clue := args[0]

		// TCPClient
		target := core.Ctx.FindTCPClientByHash(clue)
		if target == nil {
			target = core.Ctx.FindTCPClientByAlias(clue)
		}
		if target != nil {
			core.Ctx.CurrentTermite = nil
			core.Ctx.Current = target
			log.Success("The current interactive shell is set to: %s", core.Ctx.Current.FullDesc())
			readLineInstance.SetPrompt(color.CyanString(core.Ctx.Current.GetPrompt()))
			return
		}

		// TermiteClient
		targetTermite := core.Ctx.FindTermiteClientByHash(clue)
		if targetTermite == nil {
			targetTermite = core.Ctx.FindTermiteClientByAlias(clue)
		}
		if targetTermite != nil {
			core.Ctx.Current = nil
			core.Ctx.CurrentTermite = targetTermite
			log.Success("The current termite interactive shell is set to: %s", core.Ctx.CurrentTermite.FullDesc())
			readLineInstance.SetPrompt(color.CyanString(core.Ctx.CurrentTermite.GetPrompt()))
			return
		}

		log.Error("No such node: %s", clue)
	},
}
