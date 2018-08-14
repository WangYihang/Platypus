package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/lib/session"
	log "github.com/WangYihang/Platypus/lib/utils/log"
	"github.com/WangYihang/Platypus/lib/utils/reflection"
)

type Dispatcher struct{}

var servers map[string](*session.Server)

func init() {
	servers = make(map[string](*session.Server))
}

func PromptYesNo(message string) bool {
	fmt.Println(fmt.Sprintf("%s [Y/N]", message))
	inputReader := bufio.NewReader(os.Stdin)
	input, err := inputReader.ReadString('\n')
	if err != nil {
		log.Error("Read from stdin failed")
	}
	if strings.HasPrefix(strings.ToLower(input), "y") {
		return true
	} else if strings.HasPrefix(strings.ToLower(input), "n") {
		return false
	} else {
		return PromptYesNo(message)
	}
}

func (dispatcher Dispatcher) Exit(args []string) {
	if len(servers) > 0 && !PromptYesNo("There are listening servers, do you really want to exit?") {
		return
	}
	for _, server := range servers {
		server.Stop()
		delete(servers, server.Hash())
	}
	os.Exit(1)
}

func (dispatcher Dispatcher) Help(args []string) {
}

func (dispatcher Dispatcher) List(args []string) {
	log.Info(fmt.Sprintf("Listing %d servers", len(servers)))
	for _, server := range servers {
		fmt.Println(server.FullDesc())
	}
}

func (dispatcher Dispatcher) Run(args []string) {
	server := session.CreateServer("0.0.0.0", 4444)
	listener, err := server.Listen()
	if err != nil {
		fmt.Println(err)
	}
	servers[server.Hash()] = server
	go server.Run(listener)
}

func ParseInput(input string) (string, []string) {
	methods := reflection.GetAllMethods(Dispatcher{})
	args := strings.Split(strings.TrimSpace(input), " ")
	handled := false
	for _, method := range methods {
		if strings.ToLower(method) == strings.ToLower(args[0]) {
			handled = true
			break
		}
	}
	if !handled {
		return "Help", []string{}
	}
	return UpperCaseFirstChar(args[0]), args[1:]
}

func UpperCaseFirstChar(str string) string {
	return strings.ToUpper(str[0:1]) + str[1:]
}

func Serve() {
	inputReader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(">> ")
		input, err := inputReader.ReadString('\n')
		if err != nil {
			log.Error("Read from stdin failed")
		}
		method, args := ParseInput(input)
		reflection.Invoke(Dispatcher{}, method, args)
	}
}
