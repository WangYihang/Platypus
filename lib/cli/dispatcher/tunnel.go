package dispatcher

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
)

func (dispatcher Dispatcher) Tunnel(args []string) {
	if context.Ctx.Current == nil && context.Ctx.CurrentTermite == nil {
		log.Error("Interactive session is not set, please use `Jump` command to set the interactive Interact")
		return
	}

	if context.Ctx.CurrentTermite != nil {
		if len(args) != 3 {
			log.Error("Arguments error, use `Help Tunnel` to get more information")
			dispatcher.TunnelHelp([]string{})
			return
		}

		mode := args[0]
		host := args[1]
		port, err := strconv.ParseUint(args[2], 10, 16)

		if err != nil {
			log.Error("Invalid port: %s, use `Help Tunnel` to get more information", args[1])
			dispatcher.TunnelHelp([]string{})
			return
		}

		switch strings.ToLower(mode) {
		case "create":
			context.Ctx.CurrentTermite.CreateTunnel(host, uint16(port))
		case "delete":
			context.Ctx.CurrentTermite.DeleteTunnel(host, uint16(port))
		}
	}

	if context.Ctx.Current != nil {
		log.Error("Tunneling is not supported in plain reverse shell")
	}
}

func (dispatcher Dispatcher) TunnelHelp(args []string) {
	fmt.Println("Usage of Tunnel")
	fmt.Println("\tTunnel [Create|Delete] [Host] [Port]")
}

func (dispatcher Dispatcher) TunnelDesc(args []string) {
	fmt.Println("Tunnel")
	fmt.Println("\tStart a tunnel on local machine which connect to a port in internal network")
}
