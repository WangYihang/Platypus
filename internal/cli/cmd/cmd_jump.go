package cmd

import (
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
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
		target := core.FindTCPClientByHash(clue)
		if target == nil {
			target = core.FindTCPClientByAlias(clue)
		}
		if target != nil {
			core.Ctx.CurrentTermite = nil
			core.Ctx.Current = target
			log.Success("The current interactive shell is set to: %s", core.Ctx.Current.(*core.TCPClient).FullDesc())
			readLineInstance.SetPrompt(color.CyanString(core.Ctx.Current.(*core.TCPClient).GetPrompt()))
			return
		}

		// TermiteClient
		targetTermite := core.FindTermiteClientByHash(clue)
		if targetTermite == nil {
			targetTermite = core.FindTermiteClientByAlias(clue)
		}
		if targetTermite != nil {
			core.Ctx.Current = nil
			core.Ctx.CurrentTermite = targetTermite
			log.Success("The current termite interactive shell is set to: %s", core.Ctx.CurrentTermite.(*core.TermiteClient).FullDesc())
			readLineInstance.SetPrompt(color.CyanString(core.Ctx.CurrentTermite.(*core.TermiteClient).GetPrompt()))
			return
		}

		log.Error("No such node: %s", clue)
	},
}
