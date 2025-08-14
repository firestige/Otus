package cmd

import (
	"os"

	"firestige.xyz/otus/internal/log"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "otus-packet",
	Short: "otus-packet is a CLI tool for managing packets",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.GetLogger().WithError(err).Fatal("Application fatal error, exit with 1")
		os.Exit(1)
	}
}
