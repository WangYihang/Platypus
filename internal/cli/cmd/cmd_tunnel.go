package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/spf13/cobra"
)

var tunnelCmd = &cobra.Command{
	Use:   "Tunnel [ACTION] [MODE] [SRC_HOST] [SRC_PORT] [DST_HOST] [DST_PORT]",
	Short: "Create or delete tunnels (pull/push/dynamic/internet)",
	Args:  cobra.ExactArgs(6),
	Run: func(cmd *cobra.Command, args []string) {
		if core.Ctx.Current == nil && core.Ctx.CurrentTermite == nil {
			log.Error("Interactive session is not set, please use `Jump` to set it")
			return
		}

		if core.Ctx.CurrentTermite != nil {
			action := args[0]
			mode := args[1]
			srcHost := args[2]
			srcPort, err := strconv.ParseUint(args[3], 10, 16)
			if err != nil {
				log.Error("Invalid source port: %s", args[3])
				return
			}
			dstHost := args[4]
			dstPort, err := strconv.ParseUint(args[5], 10, 16)
			if err != nil {
				log.Error("Invalid destination port: %s", args[5])
				return
			}

			switch strings.ToLower(action) {
			case "create":
				switch strings.ToLower(mode) {
				case "pull":
					localAddress := fmt.Sprintf("%s:%d", dstHost, dstPort)
					remoteAddress := fmt.Sprintf("%s:%d", srcHost, srcPort)
					core.AddPullTunnelConfig(core.Ctx.CurrentTermite, localAddress, remoteAddress)
				case "push":
					localAddress := fmt.Sprintf("%s:%d", srcHost, srcPort)
					remoteAddress := fmt.Sprintf("%s:%d", dstHost, dstPort)
					core.AddPushTunnelConfig(core.Ctx.CurrentTermite, localAddress, remoteAddress)
				case "dynamic":
					core.Ctx.CurrentTermite.StartSocks5Server()
				case "internet":
					localAddress := fmt.Sprintf("%s:%d", srcHost, srcPort)
					remoteAddress := fmt.Sprintf("%s:%d", dstHost, dstPort)
					if _, exists := core.Ctx.Socks5Servers[localAddress]; exists {
						log.Warn("Socks5 server (%s) already exists", localAddress)
					} else {
						if err := core.StartSocks5Server(localAddress); err != nil {
							log.Error("Starting local socks5 server failed: %s", err.Error())
						} else {
							core.AddPushTunnelConfig(core.Ctx.CurrentTermite, localAddress, remoteAddress)
						}
					}
				default:
					log.Error("Invalid mode: %s, should be in {Pull, Push, Dynamic, Internet}", mode)
				}
			case "delete":
				log.Error("Tunnel delete: TBD")
			default:
				log.Error("Invalid action: %s, should be in {Create, Delete}", action)
			}
		}

		if core.Ctx.Current != nil {
			log.Error("Tunneling is not supported in plain reverse shell")
		}
	},
}
