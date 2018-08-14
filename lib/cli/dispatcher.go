package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/lib/session"
)

var commands = map[string]string{
	"exit":     "Exit context",
	"sessions": "List the connected sessions",
}

var servers []session.Server

func Help() {
	fmt.Println("Usage: ")
	for k, v := range commands {
		fmt.Println("\t", k, "\t", v)
	}
}

// CommandDispatcher dsa
//
func CommandDispatcher() {
	inputReader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(">> ")
		input, err := inputReader.ReadString('\n')
		if err != nil {
			fmt.Println("Read from stdin failed")
		}
		input = strings.TrimSpace(input)
		exitFlag := false
		switch input {
		case "exit":
			for _, server := range servers {
				server.Cleanup()
			}
			exitFlag = true
			break
		case "list":
			fmt.Println(fmt.Sprintf("Listing %d servers", len(servers)))
			for _, server := range servers {
				fmt.Println(server.Desc())
			}
		case "run":
			s := session.CreateServer("0.0.0.0", 4444)
			listener, err := s.Listen()
			if err != nil {
				fmt.Println(err)
				continue
			}
			servers = append(servers, *s)
			go s.Run(listener)
			break
		default:
			Help()
		}
		if exitFlag {
			break
		}
	}
}
