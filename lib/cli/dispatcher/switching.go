package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/model"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) Switching(args []string) {
	if len(args) != 1 {
		log.Error("Argments error, use `Help Switching` to get more Switchingrmation")
		dispatcher.SwitchingHelp([]string{})
		return
	}
	for _, server := range model.Ctx.Servers {
		for _, client := range server.Clients {
			if strings.HasPrefix(client.Hash, strings.ToLower(args[0])) {
				client.Group = !client.Group
				log.Success("[%t->%t] %s", !client.Group, client.Group, client.Desc())
				return
			}
		}
	}
	log.Error("No such node")
}

func (dispatcher Dispatcher) SwitchingHelp(args []string) {
	fmt.Println("Usage of Switching")
	fmt.Println("\tSwitching [HASH]")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
}

func (dispatcher Dispatcher) SwitchingDesc(args []string) {
	fmt.Println("Switching")
	fmt.Println("\tSwitch the interactive field of a node, allows you to interactive with it")
	fmt.Println("\tIf the current status is ON, it will turns to OFF. If OFF, turns ON")
}
