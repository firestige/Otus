package processor

import (
	"firestige.xyz/otus/internal/otus/module/capture"
	"firestige.xyz/otus/internal/otus/module/sender"
	processor "firestige.xyz/otus/internal/processor/api"
	"firestige.xyz/otus/plugin/filter/api"
)

type defaultProcessor struct {
	config  *processor.Config
	filters []api.Filter
	sender  *sender.Sender
	capture *capture.Capture
}
