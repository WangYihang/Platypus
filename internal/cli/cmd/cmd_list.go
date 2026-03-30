package cmd

import (
	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "List",
	Short: "List all listening servers and their clients",
	Run: func(cmd *cobra.Command, args []string) {
		if len(core.GetServers()) == 0 {
			log.Warn("No listening servers")
			return
		}
		log.Info("Listing %d listening servers", len(core.GetServers()))
		for _, server := range core.GetServers() {
			server.AsTable()
		}
	},
}
