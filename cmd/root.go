// Package cmd implements CLI commands using cobra framework.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	configFile string
	socketPath string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "otus",
	Short: "Otus - High-performance edge packet capture and observability system",
	Long: `Otus is a high-performance, low-resource edge network packet capture and observability system.
It captures network traffic, decodes protocols (L2-L4), parses application protocols (SIP, RTP, etc.),
and reports data to Kafka or other backends.

Features:
  - High throughput: ≥200K pps/core for full protocol parsing
  - Plugin architecture: capture, parser, processor, reporter plugins
  - Remote control: Kafka command subscription
  - Local control: CLI via Unix Domain Socket
  - Flexible deployment: physical, VM, container`,
	Version: "0.1.0",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "/etc/otus/config.yml",
		"config file path")
	rootCmd.PersistentFlags().StringVarP(&socketPath, "socket", "s", "/var/run/otus.sock",
		"daemon socket path")

	// Add subcommands
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(taskCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(reloadCmd)
}

// exitWithError prints error message and exits with code 1
func exitWithError(msg string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s: %v\n", msg, err)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	}
	os.Exit(1)
}
