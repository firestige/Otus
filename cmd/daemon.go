// Package cmd implements CLI commands.
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"firestige.xyz/otus/internal/command"
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/task"
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
  6. Handle signals for graceful shutdown (SIGTERM, SIGINT) and reload (SIGHUP)

Note: Full daemon implementation will be completed in Step 15.`,
	Run: func(cmd *cobra.Command, args []string) {
		runDaemon()
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

func runDaemon() {
	fmt.Println("Starting Otus daemon...")
	fmt.Printf("Config: %s\n", configFile)
	fmt.Printf("Socket: %s\n", socketPath)
	fmt.Printf("PID file: %s\n", pidFile)

	// Load global configuration
	// Note: This will be properly implemented in Step 15
	// For now, we'll use a minimal setup
	globalConfig, err := config.Load(configFile)
	if err != nil {
		// If config file doesn't exist, use defaults
		slog.Warn("failed to load config, using defaults", "error", err)
		globalConfig = &config.GlobalConfig{
			Agent: config.AgentConfig{
				ID: "otus-agent-001",
			},
		}
	}

	// Initialize logging
	// Basic setup - will be enhanced in Step 15
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("otus daemon starting",
		"version", "0.1.0",
		"agent_id", globalConfig.Agent.ID,
		"config", configFile,
	)

	// Create task manager
	taskManager := task.NewTaskManager(globalConfig.Agent.ID)

	// Create command handler
	// ConfigReloader will be properly implemented in Step 15
	cmdHandler := command.NewCommandHandler(taskManager, nil)

	// Create UDS server
	udsServer := command.NewUDSServer(socketPath, cmdHandler)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start UDS server in background
	go func() {
		if err := udsServer.Start(ctx); err != nil && err != context.Canceled {
			slog.Error("uds server failed", "error", err)
		}
	}()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	slog.Info("daemon started, waiting for signals or tasks")

	// Wait for signals
	for {
		sig := <-sigChan
		switch sig {
		case syscall.SIGTERM, syscall.SIGINT:
			slog.Info("received shutdown signal", "signal", sig)
			// Graceful shutdown
			cancel()

			// Stop all tasks
			slog.Info("stopping all tasks")
			if err := taskManager.StopAll(); err != nil {
				slog.Error("failed to stop tasks", "error", err)
			}

			// Stop UDS server
			slog.Info("stopping uds server")
			udsServer.Stop()

			slog.Info("daemon stopped gracefully")
			return

		case syscall.SIGHUP:
			slog.Info("received reload signal")
			// Reload configuration
			// This will be properly implemented in Step 15
			newConfig, err := config.Load(configFile)
			if err != nil {
				slog.Error("failed to reload config", "error", err)
			} else {
				slog.Info("configuration reloaded", "agent_id", newConfig.Agent.ID)
				// Note: Actual reload logic will be in Step 15
			}
		}
	}
}
