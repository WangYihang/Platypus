package dispatcher

// func (dispatcher CommandDispatcher) Help() string {
// 	methods := reflection.GetAllMethods(CommandDispatcher{})
// 	if len(args) == 0 {
// 		fmt.Println("Usage: ")
// 		fmt.Println("\tHelp [COMMANDS]")
// 		fmt.Println("Commands: ")
// 		for _, method := range methods {
// 			if strings.HasSuffix(method, "Desc") {
// 				reflection.Invoke(CommandDispatcher{}, method, []string{})
// 			}
// 		}
// 	} else {
// 		method := args[0]
// 		helpMethod := args[0] + "Help"
// 		if reflection.Contains(methods, method) && reflection.Contains(methods, helpMethod) {
// 			reflection.Invoke(CommandDispatcher{}, helpMethod, []string{})
// 		} else {
// 			log.Error("No such command, use `Help` to get more information")
// 		}
// 	}
// }
