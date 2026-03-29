package cmd

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/spf13/cobra"
)

var switchingCmd = &cobra.Command{
	Use:   "Switching [HASH]",
	Short: "Toggle GroupDispatch for a server or client",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		hash := strings.ToLower(args[0])

		for _, server := range core.Ctx.Servers {
			if strings.HasPrefix(server.Hash, hash) {
				server.GroupDispatch = !server.GroupDispatch
				for _, client := range server.GetAllTCPClients() {
					client.GroupDispatch = server.GroupDispatch
					log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
				}
				for _, client := range server.GetAllTermiteClients() {
					client.GroupDispatch = server.GroupDispatch
					log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
				}
				return
			}
		}

		for _, server := range core.Ctx.Servers {
			for _, client := range server.GetAllTCPClients() {
				if strings.HasPrefix(client.Hash, hash) {
					client.GroupDispatch = !client.GroupDispatch
					log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
					return
				}
			}
			for _, client := range server.GetAllTermiteClients() {
				if strings.HasPrefix(client.Hash, hash) {
					client.GroupDispatch = !client.GroupDispatch
					log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
					return
				}
			}
		}

		log.Error("No such node")
	},
}
