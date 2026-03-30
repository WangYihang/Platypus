package commands

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Session management commands",
}

var sessionsGatherCmd = &cobra.Command{
	Use:   "gather [session-id]",
	Short: "Gather system info from a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := apiClient.Post(fmt.Sprintf("/api/v1/sessions/%s/gather", args[0]), nil)
		if err != nil {
			return err
		}
		printJSON(data)
		return nil
	},
}

var sessionsDeleteCmd = &cobra.Command{
	Use:   "delete [session-id]",
	Short: "Delete/disconnect a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := apiClient.Delete(fmt.Sprintf("/api/client/%s", args[0]))
		if err != nil {
			return err
		}
		printJSON(data)
		return nil
	},
}

var sessionsFileSizeCmd = &cobra.Command{
	Use:   "filesize [session-id] [path]",
	Short: "Get file size on remote session",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{"path": {args[1]}}
		data, err := apiClient.Get(fmt.Sprintf("/api/v1/sessions/%s/files/size", args[0]), q)
		if err != nil {
			return err
		}
		printJSON(data)
		return nil
	},
}

var sessionsDispatchCmd = &cobra.Command{
	Use:   "dispatch [command]",
	Short: "Execute command on all GroupDispatch-enabled sessions",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := apiClient.Post("/api/v1/sessions/dispatch", map[string]interface{}{
			"command": args[0],
			"timeout": 5,
		})
		if err != nil {
			return err
		}
		printJSON(data)
		return nil
	},
}

func init() {
	sessionsCmd.AddCommand(sessionsGatherCmd, sessionsDeleteCmd, sessionsFileSizeCmd, sessionsDispatchCmd)
}
