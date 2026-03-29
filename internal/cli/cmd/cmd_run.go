package cmd

import (
	"strconv"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "Run [HOST] [PORT]",
	Short: "Start a new listening server",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		host := args[0]
		port, err := strconv.ParseUint(args[1], 10, 16)
		if err != nil {
			log.Error("Invalid port: %s", args[1])
			return
		}
		server := core.CreateTCPServer(host, uint16(port), "", false, true, "", "")
		if server != nil {
			go (*server).Run()
		}
	},
}
