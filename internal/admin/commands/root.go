package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/WangYihang/Platypus/internal/admin"
)

var (
	serverURL string
	token     string
	secret    string
	apiClient *admin.Client
)

var rootCmd = &cobra.Command{
	Use:   "platypus-admin",
	Short: "Platypus CLI client — manage sessions via Server API",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if serverURL == "" {
			return fmt.Errorf("--server is required")
		}
		apiClient = admin.NewClient(serverURL, token)
		if token == "" && secret != "" {
			if err := apiClient.Authenticate(secret); err != nil {
				return fmt.Errorf("authentication failed: %w", err)
			}
		}
		if apiClient.Token == "" {
			return fmt.Errorf("--token or --secret is required")
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", os.Getenv("PLATYPUS_SERVER"), "Server URL (e.g. http://localhost:7331)")
	rootCmd.PersistentFlags().StringVar(&token, "token", os.Getenv("PLATYPUS_TOKEN"), "Bearer token")
	rootCmd.PersistentFlags().StringVar(&secret, "secret", os.Getenv("PLATYPUS_SECRET"), "Server secret (obtains token automatically)")

	rootCmd.AddCommand(
		listCmd,
		execCmd,
		sessionsCmd,
		tunnelCmd,
	)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
