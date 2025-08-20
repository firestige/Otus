package sender

import (
	"firestige.xyz/otus/internal/otus/module/sender/api"
	"firestige.xyz/otus/plugin/reporter"
)

type Sender struct {
	config *api.Config

	reporters []reporter.Reporter
}
