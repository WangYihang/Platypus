package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/reflection"
)

func (ctx Dispatcher) Help(args []string) {
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
		help_method := args[0] + "Help"
		if reflection.Contains(methods, method) && reflection.Contains(methods, help_method) {
			reflection.Invoke(Dispatcher{}, help_method, []string{})
		} else {
			log.Error("No such command")
		}
	}
}
