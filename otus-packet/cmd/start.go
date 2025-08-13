package cmd

import (
	"firestige.xyz/otus/pkg/capture"
	"github.com/spf13/cobra"
)

var config string

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the application",
	Run: func(cmd *cobra.Command, args []string) {
		// Implementation of the start command
		capture := capture.GetInstance()
		capture.Start()
	},
}

func init() {
	startCmd.Flags().StringVarP(&config, "config", "c", "config.yaml", "Path to the configuration file")
	rootCmd.AddCommand(startCmd)
}
