package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/str"
)

func (dispatcher Dispatcher) Inspector(args []string) {
	log.Info(fmt.Sprintf("Inspectoring %d servers", len(context.Ctx.Servers)))
	for _, server := range context.Ctx.Servers {
		for _, client := range server.Clients {
			payload := "echo " + str.RandomString(0x10) + "\n"
			client.Write([]byte(payload))
			client.ReadUntil(payload)
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
