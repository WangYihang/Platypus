package dispatcher

import (
	"bufio"
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) Shell(args []string) {
	if Current == nil {
		log.Error("Interactive shell is not set, please use `jump` command to set the interactive shell")
		return
	}
	// go func() {
	for Current != nil && Current.Interactive {
		inputReader := bufio.NewReader(os.Stdin)
		command, err := inputReader.ReadString('\n')
		if command == "exit\n" {
			log.Info("Exiting interactive shell")
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

func (ctx Dispatcher) ShellHelp(args []string) {
	fmt.Println("Usage of Shell")
	fmt.Println("\tShell")
}

func (ctx Dispatcher) ShellDesc(args []string) {
	fmt.Println("Shell")
	fmt.Println("\tThis command will pop up a interactive shell")
}
