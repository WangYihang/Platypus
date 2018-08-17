package dispatcher

// import (
// 	"fmt"

// 	"github.com/WangYihang/Platypus/lib/context"
// 	"github.com/WangYihang/Platypus/lib/model/platypus"
// 	"github.com/WangYihang/Platypus/lib/util/log"
// )

// func (dispatcher Dispatcher) Command(args []string) {
// 	if context.Ctx.Current == nil {
// 		log.Error("Current session is not set, please use `Jump` command to set the interactive Command")
// 		return
// 	}

// 	if context.Ctx.Current.(platypus.PlatypusClient) {

// 	}

// 	log.Info("Commanding with %s", context.Ctx.Current.Desc())

// }

// func (dispatcher Dispatcher) CommandHelp(args []string) {
// 	fmt.Println("Usage of Command")
// 	fmt.Println("\tCommand")
// }

// func (dispatcher Dispatcher) CommandDesc(args []string) {
// 	fmt.Println("Command")
// 	fmt.Println("\tPop up a interactive session, you can communicate with it via stdin/stdout")
// }
