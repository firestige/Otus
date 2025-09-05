package processor

import (
	"context"
	"sync"

	"firestige.xyz/otus/internal/otus/event"
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	handler "firestige.xyz/otus/plugins/handler/api"
)

func NewProcessor(ctx context.Context, cfg *processor.Config) processor.Processor {
	ctx, cancel := context.WithCancel(ctx)
	p := &Processor{
		config:     cfg,
		handlers:   make([]handler.Handler, 0),
		inputs:     make([]chan *event.EventContext, cfg.CommonFields.Partition),
		outputs:    make([]chan *event.EventContext, cfg.CommonFields.Partition),
		partitions: make([]*partition, cfg.CommonFields.Partition),
		ctx:        ctx,
		cancel:     cancel,
		wg:         &sync.WaitGroup{},
	}
	for _, handlerCfg := range cfg.HandlerConfigs {
		p.handlers = append(p.handlers, handler.GetHandler(handlerCfg))
	}
	return p
}
