package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the service",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		if err := cli.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop: %w", err)
		}
		fmt.Println("✓ Service stopped successfully")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
