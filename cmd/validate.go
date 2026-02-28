// Package cmd implements CLI commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"icc.tech/capture-agent/internal/config"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a task configuration file",
	Long: `Validate a task configuration file (JSON or YAML) without creating a task.

This is useful for pre-checking configuration before deploying to the daemon.
File format is auto-detected from extension (.json, .yaml, .yml).

Examples:
  capture-agent validate -f task.json
  capture-agent validate -f task.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		runValidateCommand()
	},
}

var validateConfigFile string

func init() {
	validateCmd.Flags().StringVarP(&validateConfigFile, "file", "f", "",
		"task configuration file to validate (required)")
	validateCmd.MarkFlagRequired("file")
}

func runValidateCommand() {
	data, err := os.ReadFile(validateConfigFile)
	if err != nil {
		exitWithError(fmt.Sprintf("failed to read file %s", validateConfigFile), err)
	}

	taskConfig, err := config.ParseTaskConfigAuto(data, validateConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "INVALID: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("VALID: Task %q — %d parser(s), %d processor(s), %d reporter(s)\n",
		taskConfig.ID,
		len(taskConfig.Parsers),
		len(taskConfig.Processors),
		len(taskConfig.Reporters),
	)
}
