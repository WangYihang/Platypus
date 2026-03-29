package cmd

import (
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/spf13/cobra"
)

var exitCmd = &cobra.Command{
	Use:   "Exit",
	Short: "Exit Platypus",
	Run: func(cmd *cobra.Command, args []string) {
		core.Shutdown()
	},
}
