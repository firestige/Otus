// Package main is the entry point for the Otus edge packet capture agent.
package main

import (
	"fmt"
	"os"

	"firestige.xyz/otus/cmd"
	_ "firestige.xyz/otus/plugins" // 触发所有内置插件 init() 注册
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
