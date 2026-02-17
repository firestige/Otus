// Package cmd implements CLI commands.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"firestige.xyz/otus/internal/command"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	Long: `Query the Otus daemon for its overall status.

Shows: version, uptime, number of running tasks, and their IDs.`,
	Run: func(cmd *cobra.Command, args []string) {
		runStatusCommand()
	},
}

func runStatusCommand() {
	client := command.NewUDSClient(socketPath, 10*time.Second)
	ctx := context.Background()

	// Ping to check daemon is alive
	if err := client.Ping(ctx); err != nil {
		exitWithError("daemon is not running or socket is inaccessible", err)
	}

	// Get daemon status
	resp, err := client.Call(ctx, "daemon_status", nil)
	if err != nil {
		exitWithError("failed to query daemon status", err)
	}

	if resp.Error != nil {
		exitWithError(fmt.Sprintf("daemon_status failed: %s", resp.Error.Message), nil)
	}

	resultJSON, err := json.MarshalIndent(resp.Result, "", "  ")
	if err != nil {
		exitWithError("failed to format result", err)
	}

	fmt.Println(string(resultJSON))
}
