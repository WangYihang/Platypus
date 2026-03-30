package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Tunnel management commands",
}

var tunnelListCmd = &cobra.Command{
	Use:   "list [session-id]",
	Short: "List tunnels for a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := apiClient.Get(fmt.Sprintf("/api/v1/sessions/%s/tunnels", args[0]), nil)
		if err != nil {
			return err
		}
		printJSON(data)
		return nil
	},
}

var tunnelCreateCmd = &cobra.Command{
	Use:   "create [session-id]",
	Short: "Create a tunnel",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mode, _ := cmd.Flags().GetString("mode")
		src, _ := cmd.Flags().GetString("src")
		dst, _ := cmd.Flags().GetString("dst")
		data, err := apiClient.Post(fmt.Sprintf("/api/v1/sessions/%s/tunnels", args[0]), map[string]string{
			"mode":        mode,
			"src_address": src,
			"dst_address": dst,
		})
		if err != nil {
			return err
		}
		printJSON(data)
		return nil
	},
}

func init() {
	tunnelCreateCmd.Flags().String("mode", "pull", "Tunnel mode: pull, push, dynamic, internet")
	tunnelCreateCmd.Flags().String("src", "", "Source address (host:port)")
	tunnelCreateCmd.Flags().String("dst", "", "Destination address (host:port)")
	tunnelCmd.AddCommand(tunnelListCmd, tunnelCreateCmd)
}
