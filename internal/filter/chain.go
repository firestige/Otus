package filter

import otus "firestige.xyz/otus/internal/otus/api"

type FilterChain struct {
	filters []otus.Filter
	handler func(exchange *otus.Exchange)
	current otus.Filter
	chain   *FilterChain
}

func NewFilterChain(handler func(exchange *otus.Exchange), filters []otus.Filter) *FilterChain {
	allFilters := make([]otus.Filter, len(filters))
	copy(allFilters, filters)
	chain := initChain(allFilters, handler)
	return &FilterChain{
		filters: allFilters,
		handler: handler,
		chain:   chain.chain,
		current: chain.current,
	}
}

func newChain(filters []otus.Filter, handler func(exchange *otus.Exchange), current otus.Filter, chain *FilterChain) *FilterChain {
	return &FilterChain{
		filters: filters,
		handler: handler,
		current: current,
		chain:   chain,
	}
}

func initChain(filters []otus.Filter, handler func(exchange *otus.Exchange)) *FilterChain {
	chain := newChain(filters, handler, nil, nil)
	for i := len(filters) - 1; i >= 0; i-- {
		chain = newChain(filters, handler, filters[i], chain)
	}
	return chain
}

func (c *FilterChain) GetFilters() []otus.Filter {
	return c.filters
}

func (c *FilterChain) GetHandler() func(exchange *otus.Exchange) {
	return c.handler
}

func (c *FilterChain) Filter(exchange *otus.Exchange) {
	if c.current != nil && c.chain != nil {
		c.current.Filter(exchange, c.chain)
	} else {
		c.handler(exchange)
	}
}
