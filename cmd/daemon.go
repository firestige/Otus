// Package cmd implements CLI commands.
package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"firestige.xyz/otus/internal/daemon"
)

// daemonCmd represents the daemon command
var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run Otus daemon in foreground",
	Long: `Run the Otus daemon process in foreground.

The daemon will:
  1. Load global configuration from config file
  2. Initialize logging and metrics
  3. Start UDS server for CLI control
  4. Start Kafka command consumer (if configured)
  5. Wait for tasks to be created via CLI or Kafka
  6. Handle signals for graceful shutdown (SIGTERM, SIGINT) and reload (SIGHUP)`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runDaemon(); err != nil {
			slog.Error("daemon failed", "error", err)
			os.Exit(1)
		}
	},
}

var (
	daemonForeground bool
	pidFile          string
)

func init() {
	daemonCmd.Flags().BoolVarP(&daemonForeground, "foreground", "f", true,
		"run in foreground (default: true)")
	daemonCmd.Flags().StringVarP(&pidFile, "pidfile", "p", "/var/run/otus.pid",
		"PID file path")
}

func runDaemon() error {
	fmt.Println("Starting Otus daemon...")
	fmt.Printf("Config: %s\n", configFile)
	fmt.Printf("Socket: %s\n", socketPath)
	fmt.Printf("PID file: %s\n", pidFile)

	// Create daemon instance
	d, err := daemon.New(configFile, socketPath, pidFile)
	if err != nil {
		return fmt.Errorf("failed to create daemon: %w", err)
	}

	// Start all components
	if err := d.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Run main loop (blocks until shutdown)
	return d.Run()
}
