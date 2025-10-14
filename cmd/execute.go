package cmd

import (
	"fmt"
	"os"

	"firestige.xyz/otus/internal/daemon"
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/rpc"
	"github.com/spf13/cobra"
)

var (
	// 使用接口类型
	cli ClientInterface
)

var rootCmd = &cobra.Command{
	Use:   "otus",
	Short: "A CLI tool to manage otus daemon",
	Long: `otus is a command-line interface for controlling the otus background service.
It automatically manages the daemon lifecycle and provides various control commands.`,
	PersistentPreRunE: ensureDaemonAndConnect,
	PersistentPostRun: closeClient,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.GetLogger().WithError(err).Fatal("Application fatal error, exit with 1")
		os.Exit(1)
	}
}

func ensureDaemonAndConnect(cmd *cobra.Command, args []string) error {
	// daemon-stop 和 start --foreground 不需要连接
	if cmd.Name() == "daemon-stop" || (cmd.Name() == "start" && cmd.Flag("foreground").Value.String() == "true") {
		return nil
	}

	// 确保守护进程运行
	if err := daemon.EnsureDaemonRunning(); err != nil {
		return fmt.Errorf("failed to ensure daemon: %w", err)
	}

	// 建立客户端连接
	var err error
	cli, err = rpc.NewClient()
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}

	return nil
}

func closeClient(cmd *cobra.Command, args []string) {
	if cli != nil {
		cli.Close()
	}
}

// SetClient 用于测试时注入 mock 客户端
func SetClient(c ClientInterface) {
	cli = c
}

// GetClient 用于测试时获取当前客户端
func GetClient() ClientInterface {
	return cli
}
