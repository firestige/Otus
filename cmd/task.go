// Package cmd implements CLI commands.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"icc.tech/capture-agent/internal/command"
	"icc.tech/capture-agent/internal/config"
)

// taskCmd represents the task command group
var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage capture tasks",
	Long: `Manage packet capture tasks on the capture-agent daemon.

Subcommands:
  create  - Create a new capture task
  delete  - Delete a running task
  list    - List all tasks
  status  - Get task status`,
}

// taskCreateCmd represents the task create command
var taskCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new capture task",
	Long: `Create a new packet capture task from a JSON or YAML configuration file.
File format is auto-detected from extension (.json, .yaml, .yml).

Examples:
  capture-agent task create -f task.json
  capture-agent task create -f task.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		runTaskCreate(cmd)
	},
}

// taskDeleteCmd represents the task delete command
var taskDeleteCmd = &cobra.Command{
	Use:   "delete <task-id>",
	Short: "Delete a running task",
	Long:  `Delete (stop) a running packet capture task by ID.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runTaskDelete(args[0])
	},
}

// taskListCmd represents the task list command
var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tasks",
	Long:  `List all running packet capture tasks.`,
	Run: func(cmd *cobra.Command, args []string) {
		runTaskList()
	},
}

// taskStatusCmd represents the task status command
var taskStatusCmd = &cobra.Command{
	Use:   "status [task-id]",
	Short: "Get task status",
	Long: `Get the status of one or all tasks.

If task-id is provided, shows detailed status for that task.
If no task-id is provided, shows status of all tasks.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var taskID string
		if len(args) > 0 {
			taskID = args[0]
		}
		runTaskStatus(taskID)
	},
}

var (
	taskConfigFile string
)

func init() {
	// Add subcommands to task command
	taskCmd.AddCommand(taskCreateCmd)
	taskCmd.AddCommand(taskDeleteCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskStatusCmd)

	// Flags for task create
	taskCreateCmd.Flags().StringVarP(&taskConfigFile, "file", "f", "",
		"task configuration file (JSON or YAML) (required)")
	taskCreateCmd.MarkFlagRequired("file")
}

func runTaskCreate(cmd *cobra.Command) {
	// Read task config file
	data, err := os.ReadFile(taskConfigFile)
	if err != nil {
		exitWithError(fmt.Sprintf("failed to read config file %s", taskConfigFile), err)
	}

	// Parse task config — auto-detect JSON/YAML from file extension
	taskConfig, err := config.ParseTaskConfigAuto(data, taskConfigFile)
	if err != nil {
		exitWithError("failed to parse task config", err)
	}

	// Create UDS client
	client := command.NewUDSClient(socketPath, 30*time.Second)
	ctx := context.Background()

	// Send create command
	fmt.Printf("Creating task %s...\n", taskConfig.ID)
	params := command.TaskCreateParams{Config: *taskConfig}
	resp, err := client.TaskCreate(ctx, params)
	if err != nil {
		exitWithError("failed to send create command", err)
	}

	if resp.Error != nil {
		exitWithError(fmt.Sprintf("task_create failed: %s", resp.Error.Message), nil)
	}

	fmt.Printf("Task %s created successfully.\n", taskConfig.ID)
}

func runTaskDelete(taskID string) {
	client := command.NewUDSClient(socketPath, 10*time.Second)
	ctx := context.Background()

	// Send delete command
	fmt.Printf("Deleting task %s...\n", taskID)
	resp, err := client.TaskDelete(ctx, taskID)
	if err != nil {
		exitWithError("failed to send delete command", err)
	}

	if resp.Error != nil {
		exitWithError(fmt.Sprintf("task_delete failed: %s", resp.Error.Message), nil)
	}

	fmt.Printf("Task %s deleted successfully.\n", taskID)
}

func runTaskList() {
	client := command.NewUDSClient(socketPath, 10*time.Second)
	ctx := context.Background()

	// Send list command
	resp, err := client.TaskList(ctx)
	if err != nil {
		exitWithError("failed to send list command", err)
	}

	if resp.Error != nil {
		exitWithError(fmt.Sprintf("task.list failed: %s", resp.Error.Message), nil)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		exitWithError("invalid response format", nil)
	}

	tasks, ok := result["tasks"].([]interface{})
	if !ok {
		exitWithError("invalid task list format", nil)
	}

	if len(tasks) == 0 {
		fmt.Println("No running tasks.")
		return
	}

	fmt.Printf("Running tasks (%d):\n", len(tasks))
	for _, task := range tasks {
		fmt.Printf("  - %s\n", task)
	}
}

func runTaskStatus(taskID string) {
	client := command.NewUDSClient(socketPath, 10*time.Second)
	ctx := context.Background()

	// Send status command
	resp, err := client.TaskStatus(ctx, taskID)
	if err != nil {
		exitWithError("failed to send status command", err)
	}

	if resp.Error != nil {
		exitWithError(fmt.Sprintf("task.status failed: %s", resp.Error.Message), nil)
	}

	// Pretty print the result
	resultJSON, err := json.MarshalIndent(resp.Result, "", "  ")
	if err != nil {
		exitWithError("failed to format result", err)
	}

	fmt.Println(string(resultJSON))
}
