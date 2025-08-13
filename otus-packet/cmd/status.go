package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"firestige.xyz/otus/pkg/capture"
	"github.com/spf13/cobra"
)

var (
	intervalFlag string
	headlessFlag bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the application",
	Long: `Show the status of the packet capture application.

Examples:
  otus status                    # Show current status once
  otus status -t 2s              # Refresh every 2 seconds
  otus status -t 1m              # Refresh every 1 minute
  otus status -t 5s -l           # Refresh every 5 seconds in headless mode`,
	Run: func(cmd *cobra.Command, args []string) {
		nc := capture.GetInstance()

		// 解析时间间隔
		var interval time.Duration
		var refresh bool

		if intervalFlag != "" {
			var err error
			interval, err = parseInterval(intervalFlag)
			if err != nil {
				fmt.Printf("Error parsing interval '%s': %v\n", intervalFlag, err)
				os.Exit(1)
			}
			refresh = true
		}

		if refresh {
			// 设置信号处理，支持 Ctrl+C 退出
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				<-sigChan
				fmt.Println("\nExiting status monitor...")
				os.Exit(0)
			}()

			// 持续刷新模式
			nc.Status(true, interval, headlessFlag)
		} else {
			// 单次显示模式
			nc.Status(false, 0, headlessFlag)
		}
	},
}

// parseInterval 解析时间间隔字符串 (如 "2s", "1m", "30s")
func parseInterval(intervalStr string) (time.Duration, error) {
	if len(intervalStr) < 2 {
		return 0, fmt.Errorf("invalid interval format: %s", intervalStr)
	}

	// 获取数值部分和单位部分
	numStr := intervalStr[:len(intervalStr)-1]
	unit := strings.ToLower(intervalStr[len(intervalStr)-1:])

	// 解析数值
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid number in interval: %s", numStr)
	}

	if num <= 0 {
		return 0, fmt.Errorf("interval must be positive: %d", num)
	}

	// 解析单位
	switch unit {
	case "s":
		return time.Duration(num) * time.Second, nil
	case "m":
		return time.Duration(num) * time.Minute, nil
	case "h":
		return time.Duration(num) * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported time unit: %s (supported: s, m, h)", unit)
	}
}

func init() {
	statusCmd.Flags().StringVarP(&intervalFlag, "time", "t", "", "Refresh interval (e.g., 2s, 1m, 5h)")
	statusCmd.Flags().BoolVarP(&headlessFlag, "headless", "l", false, "Run in headless mode (no headers/decorations)")

	rootCmd.AddCommand(statusCmd)
}
