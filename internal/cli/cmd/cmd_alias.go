package cmd

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var aliasCmd = &cobra.Command{
	Use:   "Alias [NAME]",
	Short: "Set alias for the current session",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
			log.Error("Interactive session is not set, please use `Jump` to set it")
			return
		}
		name := strings.TrimSpace(args[0])
		if core.Ctx.Current != nil {
			log.Info("Renaming session: %s", core.Ctx.Current.(*core.TCPClient).FullDesc())
			core.Ctx.Current.(*core.TCPClient).Alias = name
			readLineInstance.SetPrompt(color.CyanString(core.Ctx.Current.(*core.TCPClient).GetPrompt()))
			return
		}
		if core.Ctx.CurrentTermite != nil {
			log.Info("Renaming session: %s", core.Ctx.CurrentTermite.(*core.TermiteClient).FullDesc())
			core.Ctx.CurrentTermite.(*core.TermiteClient).Alias = name
			readLineInstance.SetPrompt(color.CyanString(core.Ctx.CurrentTermite.(*core.TermiteClient).GetPrompt()))
		}
	},
}
