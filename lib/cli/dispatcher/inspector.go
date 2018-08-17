package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/str"
)

func (dispatcher Dispatcher) Inspector(args []string) {
	// Make sure banner be printed only once
	banner := true
	for _, server := range context.Ctx.Servers {
		for _, client := range server.Clients {
			if banner {
				log.Info(fmt.Sprintf("Inspectoring %d servers", len(context.Ctx.Servers)))
				banner = false
			}
			payload := "echo " + str.RandomString(0x10) + "\n"
			client.Write([]byte(payload))
		}
	}
}

func (dispatcher Dispatcher) InspectorHelp(args []string) {
	fmt.Println("Usage of Inspector")
	fmt.Println("\tInspector")
}

func (dispatcher Dispatcher) InspectorDesc(args []string) {
	fmt.Println("Inspector")
	fmt.Println("\tCheck all client online or not")
}
