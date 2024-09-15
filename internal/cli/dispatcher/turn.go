package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) Turn(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Turn` to get more Turnrmation")
		dispatcher.TurnHelp([]string{})
		return
	}

	hash := strings.ToLower(args[0])

	// handle the hash represent a server
	server := context.Ctx.FindServerByHash(hash)
	if server != nil {
		for _, client := range (*server).GetAllTCPClients() {
			client.GroupDispatch = !client.GroupDispatch
			log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
		}
		for _, client := range (*server).GetAllTermiteClients() {
			client.GroupDispatch = !client.GroupDispatch
			log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
		}
		return
	}

	// handle the hash represent a client
	client := context.Ctx.FindTCPClientByHash(hash)
	if client != nil {
		client.GroupDispatch = !client.GroupDispatch
		log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
	} else {
		// handle the hash represent a termite client
		termiteclient := context.Ctx.FindTermiteClientByHash(hash)
		if termiteclient != nil {
			termiteclient.GroupDispatch = !termiteclient.GroupDispatch
			log.Success("[%t->%t] %s", !termiteclient.GroupDispatch, termiteclient.GroupDispatch, termiteclient.FullDesc())
		} else {
			// handle invalid hash
			log.Error("No such node")
		}
	}
}

func (dispatcher commandDispatcher) TurnHelp(args []string) {
	fmt.Println("Usage of Turn")
	fmt.Println("\tTurn [HASH]")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
	fmt.Println("\t\tThe hash can either be the hash of an client or the hash of an server")
	fmt.Println("\t\tWhen the server: Swiching ON/OFF state of all clients related to this server")
	fmt.Println("\t\tWhen the client: Swiching ON/OFF state of the client")
}

func (dispatcher commandDispatcher) TurnDesc(args []string) {
	fmt.Println("Turn")
	fmt.Println("\tSwitch the interactive field of a node(s), allows you to interactive with it(them)")
	fmt.Println("\tIf the current status is ON, it will turns to OFF. If OFF, turns ON")
}
