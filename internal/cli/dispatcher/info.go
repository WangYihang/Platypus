package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) Info(args []string) {
	if len(args) > 1 {
		log.Error("Arguments error, use `Help Info` to get more information")
		dispatcher.InfoHelp([]string{})
		return
	}

	if len(args) == 0 {
		if context.Ctx.Current == nil && context.Ctx.CurrentTermite == nil {
			log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
			return
		}

		if context.Ctx.Current != nil {
			current := context.Ctx.Current
			current.AsTable()
			return
		}

		if context.Ctx.CurrentTermite != nil {
			current := context.Ctx.CurrentTermite
			current.AsTable()
			return
		}
	} else {
		clue := args[0]
		// Client Information
		targetClient := context.Ctx.FindTCPClientByHash(clue)
		if targetClient != nil {
			targetClient.AsTable()
			return
		}

		// Client Information
		targetTermiteClient := context.Ctx.FindTermiteClientByHash(clue)
		if targetTermiteClient != nil {
			targetTermiteClient.AsTable()
			return
		}

		// Server Information
		targetServer := context.Ctx.FindServerByHash(clue)
		if targetServer != nil {
			targetServer.AsTable()
			return
		}
		log.Error("No such node")
	}
}

func (dispatcher commandDispatcher) InfoHelp(args []string) {
	fmt.Println("Usage of Info")
	fmt.Println("\tInfo [HASH]")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
}

func (dispatcher commandDispatcher) InfoDesc(args []string) {
	fmt.Println("Info")
	fmt.Println("\tDisplay the information of a node, using the hash of the node")
}
