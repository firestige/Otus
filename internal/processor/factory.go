package processor

import "firestige.xyz/otus/internal/processor/api"

func NewProcessor() api.Processor {
	return defaultProcessor{}
}
