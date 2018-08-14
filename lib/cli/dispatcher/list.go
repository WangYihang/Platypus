package dispatcher

import (
	"fmt"

	"github.com/WangYihang/Platypus/lib/util/log"
)

func (ctx Dispatcher) List(args []string) {
	log.Info(fmt.Sprintf("Listing %d servers", len(Servers)))
	for _, server := range Servers {
		fmt.Println(server.FullDesc())
	}
}

func (ctx Dispatcher) ListHelp(args []string) {

}

func (ctx Dispatcher) ListDesc(args []string) {

}
