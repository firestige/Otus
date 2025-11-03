package processor

import (
	"firestige.xyz/otus/internal/filter"
	otus "firestige.xyz/otus/internal/otus/api"
)

type Processor struct {
	sinks []otus.Sink
	chain otus.FilterChain
}

func NewProcessor(filters []otus.Filter, sinks []otus.Sink) *Processor {
	handler := func(exchange *otus.Exchange) {
		for _, sink := range sinks {
			sink.Send(exchange)
		}
	}
	chain := filter.NewFilterChain(handler, filters)
	return &Processor{
		chain: chain,
	}
}

func (p *Processor) Process(exchange *otus.Exchange) error {
	p.chain.Filter(exchange)
	return nil
}

func (p *Processor) Close() error {
	for _, sink := range p.sinks {
		sink.Close()
	}
	return nil
}
