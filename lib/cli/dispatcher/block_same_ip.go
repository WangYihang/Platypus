package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) BlockSameIP(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help BlockSameIP` to get more information")
		dispatcher.BlockSameIPHelp([]string{})
		return
	}

	for _, server := range context.Ctx.Servers {
		if strings.HasPrefix(server.Hash(), strings.ToLower(args[0])) {
			server.BlockSameIP = !server.BlockSameIP
			log.Success("Changing `BlockSameIP` option from %t to %t", !server.BlockSameIP, server.BlockSameIP)
			return
		}
	}
	log.Error("No such node")
}

func (dispatcher Dispatcher) BlockSameIPHelp(args []string) {
	fmt.Println("Usage of BlockSameIP")
	fmt.Println("\tBlockSameIP [Server Hash]")
	fmt.Println("\tFlipping `BlockSameIP`, if true, Platypus will make sure that there is only one client from every unique IP")
	fmt.Println("\tFlipping `BlockSameIP`, if false, there is no limit on the amount of clients from every unique IP")
}

func (dispatcher Dispatcher) BlockSameIPDesc(args []string) {
	fmt.Println("BlockSameIP")
	fmt.Println("\tDecline subsequent requests from the same IP")
}
