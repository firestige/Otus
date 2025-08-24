package pipeline

import (
	"sync"

	"firestige.xyz/otus/internal/otus/module/capture"
	"firestige.xyz/otus/internal/otus/module/sender"
)

func NewPipeline(cfg *Config) Pipeline {
	pipe := &pipe{
		config: cfg,
		mu:     &sync.RWMutex{},
	}

	capture := capture.NewCapture(cfg.CaptureConfig)
	sender := sender.NewSender(cfg.SenderConfig)

	pipe.SetCapture(capture)
	pipe.SetSender(sender)

	pipe.PostConstruct()

	return pipe
}
