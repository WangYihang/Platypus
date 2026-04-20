package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all servers and sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		// List servers
		serversData, err := apiClient.Get("/api/server", nil)
		if err != nil {
			return fmt.Errorf("list servers: %w", err)
		}
		fmt.Println("=== Servers ===")
		printJSON(serversData)

		// List clients
		clientsData, err := apiClient.Get("/api/client", nil)
		if err != nil {
			return fmt.Errorf("list clients: %w", err)
		}
		fmt.Println("\n=== Sessions ===")
		printJSON(clientsData)

		return nil
	},
}

func printJSON(data []byte) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		fmt.Println(string(data))
		return
	}
	pretty, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(pretty))
}
