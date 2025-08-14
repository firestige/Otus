package cmd

import (
	"fmt"
	"os"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Otus Packet server",
	Run: func(cmd *cobra.Command, args []string) {
		data, err := os.ReadFile("/tmp/otus.pid")
		if err != nil {
			panic(err)
		}
		pid, err := strconv.Atoi(string(data))
		if err != nil {
			fmt.Print("invalid pid file")
			return
		}
		err = syscall.Kill(pid, syscall.SIGTERM)
		if err != nil {
			panic(err)
		}
		fmt.Println("Stopped Otus Packet server with PID:", pid)
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
