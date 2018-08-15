package dispatcher

import (
	"bufio"
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) Interact(args []string) {
	if context.Current == nil {
		log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
		return
	}
	log.Info("Interacting with %s", context.Current.Desc())
	// write to socket fd
	inputChannel := make(chan []byte, 1024)
	go func() {
		for {
			select {
			case data := <-inputChannel:
				if context.Current == nil {
					return
				}
				context.Current.Conn.Write(data)
			}
		}
	}()
	// read from socket fd
	go func() {
		for {
			buffer := make([]byte, 1024)
			if context.Current == nil {
				return
			}
			n, err := context.Current.Conn.Read(buffer)
			if err != nil {
				log.Error("Read failed from %s , error message: %s", context.Current.Desc(), err)
				// Clean up
				for _, server := range context.Servers {
					for _, client := range server.Clients {
						if client.Hash == context.Current.Hash {
							server.DeleteClient(client)
						}
					}
				}
				// Set Current to nil
				context.Current = nil
				break
			}
			if context.Current != nil {
				fmt.Print(string(buffer[:n]))
			}
		}
	}()

	for {
		if context.Current == nil {
			return
		}
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
}

func (ctx Dispatcher) InteractHelp(args []string) {
	fmt.Println("Usage of Interact")
	fmt.Println("\tInteract")
}

func (ctx Dispatcher) InteractDesc(args []string) {
	fmt.Println("Interact")
	fmt.Println("\tThis command will pop up a interactive session, you can communicate with it via stdin/stdout")
}
