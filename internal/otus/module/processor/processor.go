package processor

import (
	"context"
	"fmt"
	"sync"

	"firestige.xyz/otus/internal/otus/event"
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	filter "firestige.xyz/otus/plugins/filter/api"
)

type Processor struct {
	config *processor.Config

	filters []filter.Filter

	inputs  []chan *event.EventContext
	outputs []chan *event.EventContext

	partitions []*partition

	ctx    context.Context
	cancel context.CancelFunc
	wg     *sync.WaitGroup
}

type partition struct {
	input   chan *event.EventContext
	output  chan *event.EventContext
	filters []filter.Filter
}

func (p *Processor) GetInputChannel(partition int) (chan *event.EventContext, error) {
	if partition < 0 || partition >= len(p.inputs) {
		return nil, fmt.Errorf("invalid partition")
	}
	return p.inputs[partition], nil
}

func (p *Processor) GetOutputChannel(partition int) (chan *event.EventContext, error) {
	if partition < 0 || partition >= len(p.outputs) {
		return nil, fmt.Errorf("invalid partition")
	}
	return p.outputs[partition], nil
}

func (p *partition) run(ctx context.Context) {
	defer close(p.output)
	defer close(p.input)

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-p.input:
			for _, filter := range p.filters {
				filter.Filter(event)
			}
			p.output <- event
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
