package main

import (
	"os"

	"firestige.xyz/otus/cmd"
)

func main() {
	if os.Getenv("OTUSD_DAEMON_MODE") == "1" {
		cmd.RunDaemon()
		return
	}
	cmd.Execute()
}
