package dispatcher

import (
	"bufio"
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) DataDispatcher(args []string) {
	fmt.Print("Input command: ")
	inputReader := bufio.NewReader(os.Stdin)
	command, err := inputReader.ReadString('\n')
	if err != nil {
		log.Error("Empty command")
		fmt.Println()
		return
	}
	n := 0
	for _, server := range Servers {
		for _, client := range server.Clients {
			if client.Interactive {
				log.Info("Executing on %s: %s", client.Desc(), command[0:len(command)-1])
				size, err := client.Conn.Write([]byte(command + "\n"))
				fmt.Println(size)
				if err != nil {
					log.Error("Write error: ", err)
					server.DeleteClient(client)
					continue
				}
				n++
			}
		}
	}
	log.Success("Execution finished, %d node DataDispatcherd", n)
}

func (ctx Dispatcher) DataDispatcherHelp(args []string) {
	fmt.Println("Usage of DataDispatcher")
	fmt.Println("\tDataDispatcher")
	fmt.Println("\tCMD\tCommand to DataDispatcher on the clients which are interactive")
}

func (ctx Dispatcher) DataDispatcherDesc(args []string) {
	fmt.Println("DataDispatcher")
	fmt.Println("\tThis command will DataDispatcher command on all clients which are interactive")
}
