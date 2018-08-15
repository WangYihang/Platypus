package dispatcher

import (
	"bufio"
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) Interact(args []string) {
	if Current == nil {
		log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
		return
	}
	log.Info("Interacting with %s", Current.Desc())
	inputChannel := make(chan []byte, 1024)
	go func() {
		for {
			select {
			case data := <-inputChannel:
				Current.Conn.Write(data)
			}
		}
	}()

	for {
		inputReader := bufio.NewReader(os.Stdin)
		command, err := inputReader.ReadString('\n')
		if err != nil {
			log.Error("Empty command")
			fmt.Println()
			return
		}
		if command == "exit\n" {
			break
		}
		inputChannel <- []byte(command)
	}
	log.Info("Detach from %s", Current.Desc())
}

func (ctx Dispatcher) InteractHelp(args []string) {
	fmt.Println("Usage of Interact")
	fmt.Println("\tInteract")
}

func (ctx Dispatcher) InteractDesc(args []string) {
	fmt.Println("Interact")
	fmt.Println("\tThis command will pop up a interactive session, you can communicate with it via stdin/stdout")
}
