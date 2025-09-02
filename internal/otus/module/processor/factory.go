package processor

import (
	"context"
	"sync"

	"firestige.xyz/otus/internal/otus/event"
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	filter "firestige.xyz/otus/plugins/filter/api"
)

func NewProcessor(ctx context.Context, cfg *processor.Config) processor.Processor {
	ctx, cancel := context.WithCancel(ctx)
	p := &Processor{
		config:     cfg,
		filters:    make([]filter.Filter, 0),
		inputs:     make([]chan *event.EventContext, cfg.CommonFields.Partition),
		outputs:    make([]chan *event.EventContext, cfg.CommonFields.Partition),
		partitions: make([]*partition, cfg.CommonFields.Partition),
		ctx:        ctx,
		cancel:     cancel,
		wg:         &sync.WaitGroup{},
	}
	for _, filterCfg := range cfg.FilterConfigs {
		p.filters = append(p.filters, filter.GetFilter(filterCfg))
	}
	return p
}
