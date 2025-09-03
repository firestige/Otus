package api

import (
	"firestige.xyz/otus/internal/otus/event"
	module "firestige.xyz/otus/internal/otus/module/api"
)

type Processor interface {
	module.Module
	GetInputChannel(partition int) chan *event.EventContext
	GetOutputChannel(partition int) chan *event.EventContext
}
