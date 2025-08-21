package processor

import "firestige.xyz/otus/internal/processor/api"

func NewProcessor(cfg *api.Config) api.Processor {
	return defaultProcessor{}
}
