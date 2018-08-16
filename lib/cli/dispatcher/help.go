package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/reflection"
)

func (dispatcher Dispatcher) Help(args []string) {
	methods := reflection.GetAllMethods(Dispatcher{})
	if len(args) == 0 {
		fmt.Println("Usage: ")
		fmt.Println("\tHelp [COMMANDS]")
		fmt.Println("Commands: ")
		for _, method := range methods {
			if strings.HasSuffix(method, "Desc") {
				reflection.Invoke(Dispatcher{}, method, []string{})
			}
		}
	} else {
		method := args[0]
		helpMethod := args[0] + "Help"
		if reflection.Contains(methods, method) && reflection.Contains(methods, helpMethod) {
			reflection.Invoke(Dispatcher{}, helpMethod, []string{})
		} else {
			log.Error("No such command, use `Help` to get more information")
		}
	}
}
