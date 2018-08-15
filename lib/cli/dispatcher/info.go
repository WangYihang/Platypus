package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) Info(args []string) {
	if len(args) != 1 {
		log.Error("Argments error, use `Help Info` to get more information")
		ctx.InfoHelp([]string{})
		return
	}
	for _, server := range context.Ctx.Servers {
		if strings.HasPrefix(server.Hash, strings.ToLower(args[0])) {
			fmt.Println("[SERVER]: \n\t", server.FullDesc())
			return
		}
		for _, client := range server.Clients {
			if strings.HasPrefix(client.Hash, strings.ToLower(args[0])) {
				fmt.Println("[CLIENT]: \n\t", client.Desc())
				return
			}
		}
	}
	log.Error("No such node")
}

func (ctx Dispatcher) InfoHelp(args []string) {
	fmt.Println("Usage of Info")
	fmt.Println("\tInfo [HASH]")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
}

func (ctx Dispatcher) InfoDesc(args []string) {
	fmt.Println("Info")
	fmt.Println("\tDisplay the infomation of a node, using the hash of the node")
}
