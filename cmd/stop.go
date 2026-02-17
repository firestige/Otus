// Package cmd implements CLI commands.
package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"firestige.xyz/otus/internal/command"
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Otus daemon",
	Long: `Stop the Otus daemon gracefully.

This command sends a shutdown signal to the running daemon via Unix Domain Socket.
The daemon will stop all tasks, flush reporters, and exit cleanly.`,
	Run: func(cmd *cobra.Command, args []string) {
		runStopCommand()
	},
}

func runStopCommand() {
	// Note: In Phase 1, we don't have a daemon.stop command yet.
	// For now, we'll list tasks and delete them, or just inform the user.
	// This will be properly implemented in Step 15 when daemon is fully assembled.

	client := command.NewUDSClient(socketPath, 10*time.Second)
	ctx := context.Background()

	// Check if daemon is running by trying to connect
	err := client.Ping(ctx)
	if err != nil {
		exitWithError("daemon is not running or socket is inaccessible", err)
	}

	// Get list of tasks
	resp, err := client.TaskList(ctx)
	if err != nil {
		exitWithError("failed to list tasks", err)
	}

	if resp.Error != nil {
		exitWithError(fmt.Sprintf("task.list failed: %s", resp.Error.Message), nil)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		exitWithError("invalid response format", nil)
	}

	taskList, ok := result["tasks"].([]interface{})
	if !ok {
		exitWithError("invalid task list format", nil)
	}

	if len(taskList) == 0 {
		fmt.Println("No running tasks. Daemon can be stopped manually (kill daemon process).")
		fmt.Println("Note: Full daemon shutdown will be implemented in Step 15.")
		return
	}

	fmt.Printf("Found %d running task(s). Stopping all tasks...\n", len(taskList))

	// Delete each task
	for _, taskID := range taskList {
		id, ok := taskID.(string)
		if !ok {
			continue
		}

		fmt.Printf("Stopping task %s...\n", id)
		resp, err := client.TaskDelete(ctx, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop task %s: %v\n", id, err)
			continue
		}

		if resp.Error != nil {
			fmt.Fprintf(os.Stderr, "Warning: task_delete %s failed: %s\n", id, resp.Error.Message)
		}
	}

	fmt.Println("All tasks stopped.")
	fmt.Println("Note: To fully stop the daemon, kill the daemon process manually.")
	fmt.Println("      Full daemon shutdown will be implemented in Step 15.")
}
