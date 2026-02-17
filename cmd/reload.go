// Package cmd implements CLI commands.
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"firestige.xyz/otus/internal/command"
)

// reloadCmd represents the reload command
var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload the Otus daemon configuration",
	Long: `Reload the global configuration of the Otus daemon.

This command sends a config.reload signal to the running daemon via Unix Domain Socket.
The daemon will reload its global configuration file without restarting.

Note: Only global configuration is reloaded. Running tasks are not affected.`,
	Run: func(cmd *cobra.Command, args []string) {
		runReloadCommand()
	},
}

func runReloadCommand() {
	client := command.NewUDSClient(socketPath, 10*time.Second)
	ctx := context.Background()

	// Send reload command
	fmt.Println("Sending reload signal to daemon...")
	resp, err := client.ConfigReload(ctx)
	if err != nil {
		exitWithError("failed to send reload command", err)
	}

	if resp.Error != nil {
		exitWithError(fmt.Sprintf("config.reload failed: %s", resp.Error.Message), nil)
	}

	fmt.Println("Configuration reloaded successfully.")
}
