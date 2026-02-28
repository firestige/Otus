// Package main is the entry point for the capture-agent edge packet capture agent.
package main

import (
	"fmt"
	"os"

	"icc.tech/capture-agent/cmd"
	_ "icc.tech/capture-agent/plugins" // 触发所有内置插件 init() 注册
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
