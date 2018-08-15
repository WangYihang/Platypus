package dispatcher

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/reflection"
	"github.com/WangYihang/Platypus/lib/util/str"
)

type Dispatcher struct{}

func ParseInput(input string) (string, []string) {
	methods := reflection.GetAllMethods(Dispatcher{})
	args := strings.Split(strings.TrimSpace(input), " ")
	if !reflection.Contains(methods, args[0]) {
		log.Error("No such command, use `Help` to get more information")
		return "Help", []string{}
	}
	return str.UpperCaseFirstChar(args[0]), args[1:]
}

func Run() {
	inputReader := bufio.NewReader(os.Stdin)
	reflection.Invoke(Dispatcher{}, "Help", []string{})
	for {
		log.CommandPrompt(context.Ctx.CommandPrompt)
		input, err := inputReader.ReadString('\n')
		if err != nil {
			fmt.Println()
			continue
		}
		input = strings.TrimSpace(input)
		method, args := ParseInput(input)
		reflection.Invoke(Dispatcher{}, method, args)
	}
}
