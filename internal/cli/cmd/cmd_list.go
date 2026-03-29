package cmd

import (
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "List",
	Short: "List all listening servers and their clients",
	Run: func(cmd *cobra.Command, args []string) {
		if len(core.Ctx.Servers) == 0 {
			log.Warn("No listening servers")
			return
		}
		log.Info("Listing %d listening servers", len(core.Ctx.Servers))
		for _, server := range core.Ctx.Servers {
			server.AsTable()
		}
	},
}
