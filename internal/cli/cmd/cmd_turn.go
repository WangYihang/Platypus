package cmd

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/spf13/cobra"
)

var turnCmd = &cobra.Command{
	Use:   "Turn [HASH]",
	Short: "Toggle GroupDispatch for all clients under a server/client",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		hash := strings.ToLower(args[0])

		server := core.Ctx.FindServerByHash(hash)
		if server != nil {
			for _, client := range server.GetAllTCPClients() {
				client.GroupDispatch = !client.GroupDispatch
				log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
			}
			for _, client := range server.GetAllTermiteClients() {
				client.GroupDispatch = !client.GroupDispatch
				log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
			}
			return
		}

		client := core.Ctx.FindTCPClientByHash(hash)
		if client != nil {
			client.GroupDispatch = !client.GroupDispatch
			log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
			return
		}

		termiteclient := core.Ctx.FindTermiteClientByHash(hash)
		if termiteclient != nil {
			termiteclient.GroupDispatch = !termiteclient.GroupDispatch
			log.Success("[%t->%t] %s", !termiteclient.GroupDispatch, termiteclient.GroupDispatch, termiteclient.FullDesc())
			return
		}

		log.Error("No such node")
	},
}
