package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) DataDispatcher(args []string) {
	fmt.Print("Input command: ")
	inputReader := bufio.NewReader(os.Stdin)
	command, err := inputReader.ReadString('\n')
	if err != nil {
		log.Error("Empty command")
		fmt.Println()
		return
	}
	n := 0
	command = strings.TrimSpace(command)
	for _, server := range context.Ctx.Servers {
		for _, client := range (*server).GetAllTCPClients() {
			if client.GroupDispatch {
				log.Info("Executing on %s: %s", client.FullDesc(), command)
				result := client.SystemToken(command)
				log.Success("%s", result)
				n++
			}
		}
	}
	log.Success("Execution finished, %d node DataDispatcherd", n)
}

func (dispatcher Dispatcher) DataDispatcherHelp(args []string) {
	fmt.Println("Usage of DataDispatcher")
	fmt.Println("\tDataDispatcher")
}

func (dispatcher Dispatcher) DataDispatcherDesc(args []string) {
	fmt.Println("DataDispatcher")
	fmt.Println("\tDataDispatcher command on all clients which are interactive")
}
