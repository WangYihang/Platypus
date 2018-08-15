package dispatcher

import (
	"bufio"
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) Interact(args []string) {
	if Current == nil {
		log.Error("Interactive session is not set, please use `jump` command to set the interactive Interact")
		return
	}
	// go func() {
	for Current != nil && Current.Interactive {
		inputReader := bufio.NewReader(os.Stdin)
		command, err := inputReader.ReadString('\n')
		if command == "exit\n" {
			log.Info("Exiting interactive Interact")
			Current.Interactive = false
		}
		if err != nil {
			log.Error("Empty command")
			fmt.Println()
			return
		}
		_, err = Current.Conn.Write([]byte(command))
		if err != nil {
			log.Error("Write error: ", err)
			Current.Interactive = false
			Current = nil
			break
		}
	}
	// }()
	go func() {
		for Current != nil && Current.Interactive {
			buffer := make([]byte, 256)
			n, err := Current.Conn.Read(buffer)
			if err != nil {
				log.Error("Read error: ", err)
				Current.Interactive = false
				Current = nil
				break
			}
			fmt.Print(buffer[:n])
		}
	}()
}

func (ctx Dispatcher) InteractHelp(args []string) {
	fmt.Println("Usage of Interact")
	fmt.Println("\tInteract")
}

func (ctx Dispatcher) InteractDesc(args []string) {
	fmt.Println("Interact")
	fmt.Println("\tThis command will pop up a interactive session, you can communicate with it via stdin/stdout")
}
