package processor

import (
	"context"
	"sync"

	"firestige.xyz/otus/internal/otus/event"
	"firestige.xyz/otus/internal/otus/module/processor/api"
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	handler "firestige.xyz/otus/plugins/handler/api"
)

type Processor struct {
	config *processor.Config

	handlers []handler.Handler

	inputs  []chan *event.EventContext
	outputs []chan *event.EventContext

	partitions []*partition

	ctx    context.Context
	cancel context.CancelFunc
	wg     *sync.WaitGroup
}

type partition struct {
	input    chan *event.EventContext
	output   chan *event.EventContext
	handlers []handler.Handler
}

func (p *Processor) GetInputChannel(partition int) chan *event.EventContext {
	if partition < 0 || partition >= len(p.inputs) {
		return nil
	}
	return p.inputs[partition]
}

func (p *Processor) GetOutputChannel(partition int) chan *event.EventContext {
	if partition < 0 || partition >= len(p.outputs) {
		return nil
	}
	return p.outputs[partition]
}

func (p *partition) run(ctx context.Context) {
	defer close(p.output)
	defer close(p.input)

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-p.input:
			ex := api.NewExchange(event, p.output)
			for _, h := range p.handlers {
				h.Handle(ex)
			}
		}
	}

}

func (p *Processor) PostConstruct() error {

	return nil
}

func (p *Processor) Boot() {
	for _, partition := range p.partitions {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			partition.run(p.ctx)
		}()
	}
	p.wg.Wait()
}

func (p *Processor) Shutdown() {
}
