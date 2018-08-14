package dispatcher

import (
	"fmt"
)

func (ctx Dispatcher) Info(args []string) {

}

func (ctx Dispatcher) InfoHelp(args []string) {
	fmt.Println("Usage of Info")
	fmt.Println("\tInfo [TYPE] [HASH]")
	fmt.Println("\tTYPE")
	fmt.Println("\t\tS Server")
	fmt.Println("\t\tC Client")
	fmt.Println("\tHASH\tThe hash of an node, node can be both a server or a client")
}
func (ctx Dispatcher) InfoDesc(args []string) {
	fmt.Println("Info")
	fmt.Println("\tThis command will display the infomation of a node, using the hash of the node")
}
