package cmd

import (
	"firestige.xyz/otus/pkg/capture"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Otus Packet server",
	Run: func(cmd *cobra.Command, args []string) {
		// Implementation of the stop command
		capture.GetInstance().Stop()
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
