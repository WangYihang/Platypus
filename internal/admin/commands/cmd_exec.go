package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var execSessionID string

var execCmd = &cobra.Command{
	Use:   "exec [command...]",
	Short: "Execute a command on a session",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if execSessionID == "" {
			return fmt.Errorf("--session is required")
		}
		command := strings.Join(args, " ")

		data, err := apiClient.Post(
			fmt.Sprintf("/api/client/%s", execSessionID),
			map[string]string{"cmd": command},
		)
		if err != nil {
			return fmt.Errorf("exec: %w", err)
		}

		var result struct {
			Status bool   `json:"status"`
			Msg    string `json:"msg"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			fmt.Println(string(data))
			return nil
		}
		fmt.Print(result.Msg)
		return nil
	},
}

func init() {
	execCmd.Flags().StringVarP(&execSessionID, "session", "s", "", "Session hash")
}
