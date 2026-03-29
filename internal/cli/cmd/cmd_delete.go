package cmd

import (
	"strings"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "Delete [HASH|ALIAS]",
	Short: "Delete a client or server",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		clue := strings.ToLower(args[0])

		if target := core.Ctx.FindTCPClientByHash(clue); target != nil {
			log.Success("Delete client node [%s]", target.Hash)
			core.Ctx.DeleteTCPClient(target)
			return
		}
		if target := core.Ctx.FindTCPClientByAlias(clue); target != nil {
			log.Success("Delete client node [%s]", target.Hash)
			core.Ctx.DeleteTCPClient(target)
			return
		}
		if target := core.Ctx.FindTermiteClientByHash(clue); target != nil {
			log.Success("Delete encrypted client node [%s]", target.Hash)
			core.Ctx.DeleteTermiteClient(target)
			return
		}
		if target := core.Ctx.FindTermiteClientByAlias(clue); target != nil {
			log.Success("Delete encrypted client node [%s]", target.Hash)
			core.Ctx.DeleteTermiteClient(target)
			return
		}
		if target := core.Ctx.FindServerByHash(clue); target != nil {
			log.Success("Delete server node [%s]", target.Hash)
			core.Ctx.DeleteServer(target)
			return
		}
		log.Error("No such node")
	},
}
