package cli

import (
	"bufio"
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/server"
)

var commands = map[string]string{
	"exit": "Exit context",
}

func CommandDispatcher() {
	inputReader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("Please enter some input: ")
		input, err := inputReader.ReadString('\n')
		if err == nil {
			fmt.Printf("The input was: %s\n", input)
		}
		exitFlag := false
		switch input {
		case "exit\n":
			fmt.Println("exiting...")
			exitFlag = true
			break
		case "run\n":
			go server.Start("0.0.0.0", 4444)
			break
		default:
			fmt.Println("Unsupported command: ", input)
		}
		if exitFlag {
			break
		}
	}
}
