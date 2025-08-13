package cmd

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/otus"
	"github.com/spf13/cobra"
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
		configPath, err := cmd.Flags().GetString("config")
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
		if err := otus.Start(cfg, timeout); err != nil {
			panic(err)
		}
	},
}

func init() {
	startCmd.Flags().StringVarP(nil, "config", "c", "config.yaml", "Path to the configuration file")
	startCmd.Flags().StringVarP(nil, "timeout", "t", "5s", "Timeout duration for the application")
	rootCmd.AddCommand(startCmd)
}
