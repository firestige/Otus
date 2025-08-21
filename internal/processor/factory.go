package processor

import (
	"firestige.xyz/otus/internal/processor/api"
	filter "firestige.xyz/otus/plugins/filter/api"
)

func NewProcessor(cfg *api.Config) api.Processor {
	p := &Processor{
		config:  cfg,
		filters: make([]filter.Filter, 0),
	}
	for _, c := range p.config.FilterConfig {
		p.filters = append(p.filters, filter.GetFilter(c))
	}
	return p
}
