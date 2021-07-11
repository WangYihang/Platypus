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
				log.Info("Mapping remote (%s) to local (%s)", remote_address, local_address)
				context.AddTunnelConfig(context.Ctx.CurrentTermite, local_address, remote_address)
			case "push":
				// context.Ctx.CurrentTermite.CreatePushTunnel(src_host, uint16(src_port), dst_host, uint16(dst_port))
				log.Error("TBD")
			case "dynamic":
				log.Error("TBD")
			default:
				log.Error("Invalid mode: %s, should be in {'Pull', 'Push', 'Dynamic'}", mode)
			}
		case "delete":
			switch strings.ToLower(mode) {
			case "pull":
				// context.Ctx.CurrentTermite.DeletePullTunnel(dst_host, uint16(dst_port), src_host, uint16(src_port))
				log.Error("TBD")
			case "push":
				// context.Ctx.CurrentTermite.DeleteTunnel(src_host, uint16(src_port), dst_host, uint16(dst_port))
				log.Error("TBD")
			case "dynamic":
				log.Error("TBD")
			default:
				log.Error("Invalid mode: %s, should be in {'Pull', 'Push', 'Dynamic'}", mode)
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
	fmt.Println("\tTunnel [Create|Delete] [Mode] [Src Host] [Src Port] [Dst Host] [Dst Port]")
}

func (dispatcher Dispatcher) TunnelDesc(args []string) {
	fmt.Println("Tunnel")
	fmt.Println("\tStart a tunnel on local machine which connect to a port in internal network")
}
