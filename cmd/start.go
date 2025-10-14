package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/spf13/cobra"
)

var foreground bool

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the service",
	Long:  "Start the otus service and begin processing tasks.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if foreground {
			return runForeground()
		}
		return runStart(cmd.Context(), cli, cmd.OutOrStdout())
	},
}

func init() {
	startCmd.Flags().BoolVarP(&foreground, "foreground", "f", false, "Run in foreground mode (for systemd)")
}

func runStart(ctx context.Context, client ClientInterface, out io.Writer) error {
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}
	fmt.Fprintln(out, "✓ Service started successfully")
	return nil
}

func runForeground() error {
	fmt.Println("Starting in foreground mode...")

	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	return syscall.Exec(execPath, []string{execPath}, append(os.Environ(), "OTUS_DAEMON_MODE=1"))
}

func init() {
	rootCmd.AddCommand(startCmd)
}
