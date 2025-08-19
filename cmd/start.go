package cmd

import (
	"os"
	"time"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/otus/boot"
	"github.com/spf13/cobra"
)

var (
	configPath string
	timeout    time.Duration
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the application",
	Long: `
Start the Otus(Optimized Traffic Unveiling Suite) application.

Examples:
  otus start                                # Start the application by default config and shutdown timeout 5s
  otus start -c config.yml                  # Start the application by config.yml and shutdown timeout 5s
  otus start -c config.yml -t 1m            # Start the application by config.yml and shutdown timeout 1m
`,
	Run: func(cmd *cobra.Command, args []string) {
		pid := os.Getpid()
		err := os.WriteFile("/tmp/otus.pid", []byte(string(pid)), 0644)
		if err != nil {
			panic(err)
		}
		defer os.Remove("/tmp/otus.pid")

		configPath, err := cmd.Flags().GetString("config")
		if err != nil {
			panic(err)
		}
		// Load configuration
		cfg, err := config.Load(configPath)
		if err != nil {
			panic(err)
		}

		timeout, err := cmd.Flags().GetDuration("timeout")
		if err != nil {
			panic(err)
		}

		// Start the application
		if err := boot.Start(cfg, timeout); err != nil {
			panic(err)
		}
	},
}

func init() {
	startCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to the configuration file")
	startCmd.Flags().DurationVarP(&timeout, "timeout", "t", 5*time.Second, "Timeout duration for the application")
	rootCmd.AddCommand(startCmd)
}
