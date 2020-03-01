package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/fatih/color"
)

func (dispatcher Dispatcher) Jump(args []string) {
	if len(args) != 1 {
		log.Error("Arguments error, use `Help Jump` to get more information")
		dispatcher.JumpHelp([]string{})
		return
	}
	for _, server := range context.Ctx.Servers {
		for _, client := range (*server).GetAllTCPClients() {
			if strings.HasPrefix(client.Hash, strings.ToLower(args[0])) {
				context.Ctx.Current = client
				log.Success("The current interactive shell is set to: %s", client.FullDesc())

				// Update prompt
				// BUG:
				// The prompt will set only at the `Jump` command once.
				// If we jump to a client before the os & user is detected
				// So the prompt will be:
				// (Unknown) 127.0.0.1:43802 [unknown] »
				var user string
				if client.User == "" {
					user = "unknown"
				} else {
					user = client.User
				}
				ReadLineInstance.SetPrompt(color.CyanString(
					"(%s) %s [%s] » ",
					client.OS.String(),
					client.Conn.RemoteAddr().String(),
					user,
				))
				return
			}
		}
	}
	log.Error("No such node")
}

func (dispatcher Dispatcher) JumpHelp(args []string) {
	fmt.Println("Usage of Jump")
	fmt.Println("\tJump [HASH]")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
}

func (dispatcher Dispatcher) JumpDesc(args []string) {
	fmt.Println("Jump")
	fmt.Println("\tJump to a node, waiting to interactive with it")
}
