package filter

import otus "firestige.xyz/otus/internal/otus/api"

type Filter interface {
	Filter(exchange *otus.Exchange, chain otus.FilterChain)
}

type CounterFilter struct {
	count int
}

func (f *CounterFilter) Filter(exchange *otus.Exchange, chain otus.FilterChain) {
	f.count++
	chain.Filter(exchange)
}

func NewCounterFilter() *CounterFilter {
	return &CounterFilter{count: 0}
}

func (f *CounterFilter) GetCount() int {
	return f.count
}
