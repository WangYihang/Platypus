package dispatcher

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/util/log"
)

func (dispatcher CommandDispatcher) Turn(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Turn` to get more Turnrmation")
		dispatcher.TurnHelp()
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
		return
	}

	// handle the hash represent a client
	client := context.Ctx.FindTCPClientByHash(hash)
	if client != nil {
		client.GroupDispatch = !client.GroupDispatch
		log.Success("[%t->%t] %s", !client.GroupDispatch, client.GroupDispatch, client.FullDesc())
	} else {
		// handle invalid hash
		log.Error("No such node")
	}
}

func (dispatcher CommandDispatcher) TurnHelp() string {
	// fmt.Println("Usage of Turn")
	// fmt.Println("\tTurn [HASH]")
	// fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
	// fmt.Println("\t\tThe hash can either be the hash of an client or the hash of an server")
	// fmt.Println("\t\tWhen the server: Swiching ON/OFF state of all clients related to this server")
	// fmt.Println("\t\tWhen the client: Swiching ON/OFF state of the client")
	return ""
}

func (dispatcher CommandDispatcher) TurnDesc() string {
	// fmt.Println("Turn")
	// fmt.Println("\tSwitch the interactive field of a node(s), allows you to interactive with it(them)")
	// fmt.Println("\tIf the current status is ON, it will turns to OFF. If OFF, turns ON")
	return ""
}
