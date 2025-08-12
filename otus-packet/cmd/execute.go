package cmd

import (
	"os"

	loggerFactory "firestige.xyz/otus/pkg/log"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "otus-packet",
	Short: "otus-packet is a CLI tool for managing packets",
}

var log = loggerFactory.GetLogger()

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.WithError(err).Fatal("Application fatal error, exit with 1")
		os.Exit(1)
	}
}
