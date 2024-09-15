package dispatcher

import (
	"fmt"
	"strings"

	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/reflection"
)

func (dispatcher commandDispatcher) Help(args []string) {
	methods := reflection.GetAllMethods(commandDispatcher{})
	if len(args) == 0 {
		fmt.Println("Usage: ")
		fmt.Println("\tHelp [COMMANDS]")
		fmt.Println("Commands: ")
		for _, method := range methods {
			if strings.HasSuffix(method, "Desc") {
				reflection.Invoke(commandDispatcher{}, method, []string{})
			}
		}
	} else {
		method := args[0]
		helpMethod := args[0] + "Help"
		if reflection.Contains(methods, method) && reflection.Contains(methods, helpMethod) {
			reflection.Invoke(commandDispatcher{}, helpMethod, []string{})
		} else {
			log.Error("No such command, use `Help` to get more information")
		}
	}
}
