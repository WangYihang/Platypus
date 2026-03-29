package cmd

import (
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/spf13/cobra"
)

var ptyCmd = &cobra.Command{
	Use:   "PTY",
	Short: "Establish PTY on the current session",
	Run: func(cmd *cobra.Command, args []string) {
		if core.Ctx.Current == nil {
			log.Error("The current client is not set, please use `Jump` to set it")
			return
		}
		if err := core.Ctx.Current.EstablishPTY(); err != nil {
			log.Error("Establish PTY failed: %s", err)
		}
	},
}
