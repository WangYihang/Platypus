package dispatcher

import (
	"fmt"
)

func (dispatcher Dispatcher) Tunnel(args []string) {
	fmt.Println("TO BE IMPLEMENTED.")
}

func (dispatcher Dispatcher) TunnelHelp(args []string) {
	fmt.Println("Usage of Tunnel")
	fmt.Println("\tTunnel [SRC] [DST]")
}

func (dispatcher Dispatcher) TunnelDesc(args []string) {
	fmt.Println("Tunnel")
	fmt.Println("\tStart a tunnel on local machine which connect to a port in internal network")
}
