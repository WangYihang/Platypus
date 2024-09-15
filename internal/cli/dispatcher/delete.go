package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) Delete(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Delete` to get more information")
		dispatcher.DeleteHelp([]string{})
		return
	}

	clue := strings.ToLower(args[0])

	// Delete TCPClient
	target := context.Ctx.FindTCPClientByHash(clue)
	if target == nil {
		target = context.Ctx.FindTCPClientByAlias(clue)
	}
	if target != nil {
		log.Success("Delete client node [%s]", target.Hash)
		context.Ctx.DeleteTCPClient(target)
		return
	}

	// Delete TermiteClient
	targetTermite := context.Ctx.FindTermiteClientByHash(clue)
	if targetTermite == nil {
		targetTermite = context.Ctx.FindTermiteClientByAlias(clue)
	}
	if targetTermite != nil {
		log.Success("Delete encrypted client node [%s]", targetTermite.Hash)
		context.Ctx.DeleteTermiteClient(targetTermite)
		return
	}

	// Delete Server
	targetServer := context.Ctx.FindServerByHash(clue)
	if targetServer != nil {
		if targetServer.Encrypted {
			log.Success("Delete encrypted server node [%s]", targetServer.Hash)
		} else {
			log.Success("Delete server node [%s]", targetServer.Hash)
		}
		context.Ctx.DeleteServer(targetServer)
		return
	}

	log.Error("No such node")
}

func (dispatcher commandDispatcher) DeleteHelp(args []string) {
	fmt.Println("Usage of Delete")
	fmt.Println("\tDelete [HASH]")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
}

func (dispatcher commandDispatcher) DeleteDesc(args []string) {
	fmt.Println("Delete")
	fmt.Println("\tDelete a node, node can be both a server or a client")
}
