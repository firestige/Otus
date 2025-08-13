package main

import (
	"firestige.xyz/otus/cmd"
	loggerFactory "firestige.xyz/otus/pkg/log"
)

var log = loggerFactory.GetLogger()

func main() {
	cmd.Execute()
}
