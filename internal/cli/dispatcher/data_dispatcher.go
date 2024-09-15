package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) DataDispatcher(args []string) {
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

		for _, client := range (*server).GetAllTermiteClients() {
			if client.GroupDispatch {
				log.Info("Executing on %s: %s", client.FullDesc(), command)
				// Check for timeout
				c1 := make(chan string, 1)
				go func() {
					result := client.System(command)
					c1 <- result
				}()
				select {
				case result := <-c1:
					log.Success("%s", result)
				case <-time.After(3 * time.Second):
					log.Error("Command timed out %s: %s", client.FullDesc(), command)
				}
				n++
			}
		}
	}
	log.Success("Execution finished, %d node DataDispatcherd", n)
}

func (dispatcher commandDispatcher) DataDispatcherHelp(args []string) {
	fmt.Println("Usage of DataDispatcher")
	fmt.Println("\tDataDispatcher")
}

func (dispatcher commandDispatcher) DataDispatcherDesc(args []string) {
	fmt.Println("DataDispatcher")
	fmt.Println("\tDataDispatcher command on all clients which are interactive")
}
