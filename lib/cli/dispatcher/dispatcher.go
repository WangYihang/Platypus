package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/lib/session"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/reflection"
	"github.com/WangYihang/Platypus/lib/util/str"
)

type Dispatcher struct{}

var Servers map[string](*session.Server)

var command_prompt = ">> "

func init() {
	Servers = make(map[string](*session.Server))
}

func ParseInput(input string) (string, []string) {
	methods := reflection.GetAllMethods(Dispatcher{})
	args := strings.Split(strings.TrimSpace(input), " ")
	if !reflection.Contains(methods, args[0]) {
		return "Help", []string{}
	}
	return str.UpperCaseFirstChar(args[0]), args[1:]
}

func Serve() {
	inputReader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(command_prompt)
		input, err := inputReader.ReadString('\n')
		if err != nil {
			log.Error("Read from stdin failed")
		}
		method, args := ParseInput(input)
		reflection.Invoke(Dispatcher{}, method, args)
	}
}
