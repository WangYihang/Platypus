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
		if len(args) != 6 {
			log.Error("Arguments error, use `Help Tunnel` to get more information")
			dispatcher.TunnelHelp([]string{})
			return
		}

		action := args[0]
		mode := args[1]
		src_host := args[2]
		src_port, err := strconv.ParseUint(args[3], 10, 16)

		if err != nil {
			log.Error("Invalid port: %s, use `Help Tunnel` to get more information", args[1])
			dispatcher.TunnelHelp([]string{})
			return
		}

		dst_host := args[4]
		dst_port, err := strconv.ParseUint(args[5], 10, 16)

		if err != nil {
			log.Error("Invalid port: %s, use `Help Tunnel` to get more information", args[1])
			dispatcher.TunnelHelp([]string{})
			return
		}

		switch strings.ToLower(action) {
		case "create":
			switch strings.ToLower(mode) {
			case "pull":
				local_address := fmt.Sprintf("%s:%d", dst_host, dst_port)
				remote_address := fmt.Sprintf("%s:%d", src_host, src_port)
				context.AddPullTunnelConfig(context.Ctx.CurrentTermite, local_address, remote_address)
			case "push":
				local_address := fmt.Sprintf("%s:%d", src_host, src_port)
				remote_address := fmt.Sprintf("%s:%d", dst_host, dst_port)
				context.AddPushTunnelConfig(context.Ctx.CurrentTermite, local_address, remote_address)
			case "dynamic":
				context.Ctx.CurrentTermite.StartSocks5Server()
			case "internet":
				local_address := fmt.Sprintf("%s:%d", src_host, src_port)
				remote_address := fmt.Sprintf("%s:%d", dst_host, dst_port)
				if _, exists := context.Ctx.Socks5Servers[local_address]; exists {
					log.Warn("Socks5 server (%s) already exists", local_address)
				} else {
					err := context.StartSocks5Server(local_address)
					if err != nil {
						log.Error("Starting local socks5 server failed: %s", err.Error())
					} else {
						context.AddPushTunnelConfig(context.Ctx.CurrentTermite, local_address, remote_address)
					}
				}
			default:
				log.Error("Invalid mode: %s, should be in {'Pull', 'Push', 'Dynamic', 'Internet'}", mode)
			}
		case "delete":
			switch strings.ToLower(mode) {
			case "pull":
				log.Error("TBD")
			case "push":
				log.Error("TBD")
			case "dynamic":
				log.Error("TBD")
			case "internet":
				log.Error("TBD")
			default:
				log.Error("Invalid mode: %s, should be in {'Pull', 'Push', 'Dynamic', 'Internet'}", mode)
			}
		default:
			log.Error("Invalid action: %s, should be in {'Create', 'Delete'}", action)
		}
	}

	if context.Ctx.Current != nil {
		log.Error("Tunneling is not supported in plain reverse shell")
	}
}

func (dispatcher Dispatcher) TunnelHelp(args []string) {
	fmt.Println("Usage of Tunnel")
	fmt.Println("\tTunnel [Create|Delete] [Pull|Push|Dynamic|Internet] [Src Host] [Src Port] [Dst Host] [Dst Port]")
}

func (dispatcher Dispatcher) TunnelDesc(args []string) {
	fmt.Println("Tunnel")
	fmt.Println("\tStart a tunnel on local machine which connect to a port in internal network")
}
