// Package main is the entry point for the Otus edge packet capture agent.
package main

import (
	"fmt"
	"os"

	"firestige.xyz/otus/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
