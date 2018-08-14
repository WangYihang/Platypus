package dispatcher

import (
	"bufio"
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) Execute(args []string) {
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
	log.Success("Execution finished, %d node executed", n)
}

func (ctx Dispatcher) ExecuteHelp(args []string) {
	fmt.Println("Usage of Execute")
	fmt.Println("\tExecute")
	fmt.Println("\tCMD\tCommand to execute on the clients which are interactive")
}

func (ctx Dispatcher) ExecuteDesc(args []string) {
	fmt.Println("Execute")
	fmt.Println("\tThis command will execute command on all clients which are interactive")
}
