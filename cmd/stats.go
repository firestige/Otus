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

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show runtime statistics",
	Long: `Query the Otus daemon for runtime statistics.

Shows: capture rates, drop counts, pipeline throughput per task.`,
	Run: func(cmd *cobra.Command, args []string) {
		runStatsCommand()
	},
}

func runStatsCommand() {
	client := command.NewUDSClient(socketPath, 10*time.Second)
	ctx := context.Background()

	resp, err := client.Call(ctx, "daemon_stats", nil)
	if err != nil {
		exitWithError("failed to query stats", err)
	}

	if resp.Error != nil {
		exitWithError(fmt.Sprintf("daemon_stats failed: %s", resp.Error.Message), nil)
	}

	resultJSON, err := json.MarshalIndent(resp.Result, "", "  ")
	if err != nil {
		exitWithError("failed to format result", err)
	}

	fmt.Println(string(resultJSON))
}
