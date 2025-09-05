package api

import (
	"firestige.xyz/otus/internal/otus/event"
	module "firestige.xyz/otus/internal/otus/module/api"
)

type Exchange struct {
	ctx    *event.EventContext
	output chan *event.EventContext
}

func NewExchange(ctx *event.EventContext, output chan *event.EventContext) *Exchange {
	return &Exchange{
		ctx:    ctx,
		output: output,
	}
}

func (e *Exchange) GetEvent() *event.EventContext {
	return e.ctx
}

func (e *Exchange) CopyWith(ctx *event.EventContext) *Exchange {
	return &Exchange{
		ctx:    ctx,
		output: e.output,
	}
}

func (e *Exchange) Submit() {
	e.output <- e.ctx
}

type HandleFunc func(exchange *Exchange)

type Processor interface {
	module.Module
	GetInputChannel(partition int) chan *event.EventContext
	GetOutputChannel(partition int) chan *event.EventContext
}
