package dispatcher

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/internal/context"
	"github.com/WangYihang/Platypus/internal/utils/log"
)

func (dispatcher commandDispatcher) Tunnel(args []string) {
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
		srcHost := args[2]
		srcPort, err := strconv.ParseUint(args[3], 10, 16)

		if err != nil {
			log.Error("Invalid port: %s, use `Help Tunnel` to get more information", args[1])
			dispatcher.TunnelHelp([]string{})
			return
		}

		dstHost := args[4]
		dstPort, err := strconv.ParseUint(args[5], 10, 16)

		if err != nil {
			log.Error("Invalid port: %s, use `Help Tunnel` to get more information", args[1])
			dispatcher.TunnelHelp([]string{})
			return
		}

		switch strings.ToLower(action) {
		case "create":
			switch strings.ToLower(mode) {
			case "pull":
				localAddress := fmt.Sprintf("%s:%d", dstHost, dstPort)
				remoteAddress := fmt.Sprintf("%s:%d", srcHost, srcPort)
				context.AddPullTunnelConfig(context.Ctx.CurrentTermite, localAddress, remoteAddress)
			case "push":
				localAddress := fmt.Sprintf("%s:%d", srcHost, srcPort)
				remoteAddress := fmt.Sprintf("%s:%d", dstHost, dstPort)
				context.AddPushTunnelConfig(context.Ctx.CurrentTermite, localAddress, remoteAddress)
			case "dynamic":
				context.Ctx.CurrentTermite.StartSocks5Server()
			case "internet":
				localAddress := fmt.Sprintf("%s:%d", srcHost, srcPort)
				remoteAddress := fmt.Sprintf("%s:%d", dstHost, dstPort)
				if _, exists := context.Ctx.Socks5Servers[localAddress]; exists {
					log.Warn("Socks5 server (%s) already exists", localAddress)
				} else {
					err := context.StartSocks5Server(localAddress)
					if err != nil {
						log.Error("Starting local socks5 server failed: %s", err.Error())
					} else {
						context.AddPushTunnelConfig(context.Ctx.CurrentTermite, localAddress, remoteAddress)
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

func (dispatcher commandDispatcher) TunnelHelp(args []string) {
	fmt.Println("Usage of Tunnel")
	fmt.Println("\tTunnel [Create|Delete] [Pull|Push|Dynamic|Internet] [Src Host] [Src Port] [Dst Host] [Dst Port]")
}

func (dispatcher commandDispatcher) TunnelDesc(args []string) {
	fmt.Println("Tunnel")
	fmt.Println("\tStart a tunnel on local machine which connect to a port in internal network")
}
