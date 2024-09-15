package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) Switching(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Switching` to get more Switchingrmation")
		dispatcher.SwitchingHelp([]string{})
		return
	}

	// handle the hash represent a server
	for _, server := range context.Ctx.Servers {
		if strings.HasPrefix(server.Hash, strings.ToLower(args[0])) {
			// flip server `GroupDispatch` state
			server.GroupDispatch = !server.GroupDispatch
			// flush all clients related to this server
			for _, client := range (*server).GetAllTCPClients() {
				client.GroupDispatch = server.GroupDispatch
				log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
			}
			for _, client := range (*server).GetAllTermiteClients() {
				client.GroupDispatch = server.GroupDispatch
				log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
			}
			return
		}
	}

	// handle the hash represent a client
	for _, server := range context.Ctx.Servers {
		for _, client := range (*server).GetAllTCPClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(args[0])) {
				client.GroupDispatch = !client.GroupDispatch
				log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
				return
			}
		}
		for _, client := range (*server).GetAllTermiteClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(args[0])) {
				client.GroupDispatch = !client.GroupDispatch
				log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
				return
			}
		}
	}

	// handle invalid hash
	log.Error("No such node")
}

func (dispatcher commandDispatcher) SwitchingHelp(args []string) {
	fmt.Println("Usage of Switching")
	fmt.Println("\tSwitching [HASH]")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
	fmt.Println("\t\tThe hash can either be the hash of an client or the hash of an server")
	fmt.Println("\t\tWhen the server: Swiching ON/OFF ALL the clients related to this server")
	fmt.Println("\t\tWhen the client: Swiching ON/OFF state of the client")
}

func (dispatcher commandDispatcher) SwitchingDesc(args []string) {
	fmt.Println("Switching")
	fmt.Println("\tSwitch the interactive field of a node(s), allows you to interactive with it(them)")
	fmt.Println("\tIf the current status is ON, it will turns to OFF. If OFF, turns ON")
}
