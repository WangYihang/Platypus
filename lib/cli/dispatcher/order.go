package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/config"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) Order(args []string) {
	log.Info(fmt.Sprintf("Ordering %d servers", len(context.Ctx.Servers)))
	for _, server := range context.Ctx.Servers {
		for _, client := range (*server).GetAllTCPClients(){
			command := config.Cfg.Section("Batch").Key("Command").MustString("/bin/cat /flag")
			log.Info("Executing: %s on %s", command, client.OnelineDesc())
			response := ((*client)).SystemToken(
				command,
			)
			log.Info("Response of %s: %s", client.OnelineDesc(), response)
		}
	} 
}

func (dispatcher Dispatcher) OrderHelp(args []string) {
	fmt.Println("Usage of Order")
	fmt.Println("\tOrder")
}

func (dispatcher Dispatcher) OrderDesc(args []string) {
	fmt.Println("Order")
	fmt.Println("\tExecute a specific command provided in runtime/app.ini")
	fmt.Println("\tAnd report the output of command to a specific server via HTTP")
}
