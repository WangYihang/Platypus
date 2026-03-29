package cmd

import (
	"fmt"

	"github.com/WangYihang/Platypus/internal/core"
	"github.com/WangYihang/Platypus/internal/log"
	oss "github.com/WangYihang/Platypus/internal/utils/os"
	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:   "Upgrade [CONNECT_BACK_ADDR]",
	Short: "Upgrade the current reverse shell to an encrypted termite client",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		connectBackAddr := args[0]
		if core.Ctx.Current == nil {
			log.Error("The current client is not set, please use `Jump` to set it")
			return
		}
		if core.Ctx.Current.OS != oss.Linux {
			log.Error("The operating system of the current client is not supported")
			return
		}
		core.Ctx.Current.UpgradeToTermite(connectBackAddr)
	},
}

var upgradeToMetasploitCmd = &cobra.Command{
	Use:   "UpgradeToMetasploit",
	Short: "Upgrade to Metasploit (not yet implemented)",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("TO BE IMPLEMENTED.")
	},
}
