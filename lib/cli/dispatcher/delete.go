package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) Delete(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Delete` to get more information")
		dispatcher.DeleteHelp([]string{})
		return
	}
	for _, server := range context.Ctx.Servers {
		if strings.HasPrefix(server.Hash, strings.ToLower(args[0])) {
			context.Ctx.DeleteServer(server)
			log.Success("Delete server node [%s]", server.Hash)
			return
		}
		for _, client := range (*server).GetAllTCPClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(args[0])) {
				context.Ctx.DeleteTCPClient(client)
				log.Success("Delete client node [%s]", client.Hash)
				return
			}
		}
	}
	log.Error("No such node")
}

func (dispatcher Dispatcher) DeleteHelp(args []string) {
	fmt.Println("Usage of Delete")
	fmt.Println("\tDelete [HASH]")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
}

func (dispatcher Dispatcher) DeleteDesc(args []string) {
	fmt.Println("Delete")
	fmt.Println("\tDelete a node, node can be both a server or a client")
}
