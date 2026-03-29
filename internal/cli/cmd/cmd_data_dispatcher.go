package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/spf13/cobra"
)

var dataDispatcherCmd = &cobra.Command{
	Use:   "DataDispatcher",
	Short: "Execute a command on all clients with GroupDispatch enabled",
	Run: func(cmd *cobra.Command, args []string) {
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
		for _, server := range core.Ctx.Servers {
			for _, client := range server.GetAllTCPClients() {
				if client.GroupDispatch {
					log.Info("Executing on %s: %s", client.FullDesc(), command)
					result := client.SystemToken(command)
					log.Success("%s", result)
					n++
				}
			}
			for _, client := range server.GetAllTermiteClients() {
				if client.GroupDispatch {
					log.Info("Executing on %s: %s", client.FullDesc(), command)
					c1 := make(chan string, 1)
					go func() {
						c1 <- client.System(command)
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
		log.Success("Execution finished, %d nodes dispatched", n)
	},
}
