package processor

import (
	"context"
	"fmt"

	"firestige.xyz/otus/internal/otus/module/api"
	capture "firestige.xyz/otus/internal/otus/module/capture/api"
	sender "firestige.xyz/otus/internal/otus/module/sender/api"
	processor "firestige.xyz/otus/internal/processor/api"
	filter "firestige.xyz/otus/plugins/filter/api"
)

type Processor struct {
	config  *processor.Config
	filters []filter.Filter
	sender  sender.Sender
	capture capture.Capture
}

func (p *Processor) PostConstruct() error {
	return nil
}

func (p *Processor) Boot(ctx context.Context) {
	// TODO 待补完
}

func (p *Processor) Shutdown() {
}

func (p *Processor) SetCapture(m api.Module) error {
	if c, ok := m.(capture.Capture); ok {
		p.capture = c
		return nil
	}
	return fmt.Errorf("invalid capture module")
}

func (p *Processor) SetSender(m api.Module) error {
	if s, ok := m.(sender.Sender); ok {
		p.sender = s
		return nil
	}
	return fmt.Errorf("invalid sender module")
}
