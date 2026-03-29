package cmd

import (
	"fmt"
	"strconv"

	"github.com/WangYihang/Platypus/internal/api"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/spf13/cobra"
)

var restCmd = &cobra.Command{
	Use:   "REST [HOST] [PORT]",
	Short: "Start a RESTful HTTP Server",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		host := args[0]
		port, err := strconv.Atoi(args[1])
		if err != nil {
			log.Error("Invalid port: %s", args[1])
			return
		}
		rest := api.CreateRESTfulAPIServer()
		go rest.Run(fmt.Sprintf("%s:%d", host, port))
		log.Info("RESTful HTTP Server running at %s:%d", host, port)
	},
}
