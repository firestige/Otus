// Package cmd implements CLI commands.
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"firestige.xyz/otus/internal/command"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Otus daemon",
	Long: `Stop the Otus daemon gracefully.

This command sends a daemon_shutdown signal to the running daemon via Unix Domain Socket.
The daemon will stop all tasks, flush reporters, and exit cleanly.`,
	Run: func(cmd *cobra.Command, args []string) {
		runStopCommand()
	},
}

func runStopCommand() {
	client := command.NewUDSClient(socketPath, 10*time.Second)
	ctx := context.Background()

	// Check if daemon is running
	if err := client.Ping(ctx); err != nil {
		exitWithError("daemon is not running or socket is inaccessible", err)
	}

	// Send graceful shutdown command
	fmt.Println("Sending shutdown signal to daemon...")
	resp, err := client.DaemonShutdown(ctx)
	if err != nil {
		exitWithError("failed to send shutdown command", err)
	}

	if resp.Error != nil {
		exitWithError(fmt.Sprintf("daemon_shutdown failed: %s", resp.Error.Message), nil)
	}

	fmt.Println("Daemon is shutting down gracefully.")
}
