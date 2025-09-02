package processor

import (
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	filter "firestige.xyz/otus/plugins/filter/api"
)

type Processor struct {
	config *processor.Config

	filters []filter.Filter

	inputs []<-chan otus.OutputPacketContext
}
