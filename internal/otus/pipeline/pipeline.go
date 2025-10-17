package pipeline

import (
	"context"
	"sync"

	"firestige.xyz/otus/internal/otus/module/datasource"
)

type PipelineConfig struct {
	name           string
	PartitionCount int
}

type Pipeline struct {
	cfg *PipelineConfig

	partitions []*partition

	ctx    context.Context
	cancel context.CancelFunc
	wg     *sync.WaitGroup
}

func (p *Pipeline) Name() string {
	return p.cfg.name
}

func (p *Pipeline) Init(ctx context.Context) {
	for i := 0; i < p.cfg.PartitionCount; i++ {
		part := newPartition(i, p)
		part.dataSource = datasource.NewSource(nil)
		// todo: initialize packetHandler and senders
		part.packetHandler = newPacketHandler(nil)
		p.partitions = append(p.partitions, part)
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
}

func (p *Pipeline) Start() {
	for _, part := range p.partitions {
		p.wg.Add(1)
		go func(part *partition) {
			defer p.wg.Done()
			part.Start(p.ctx)
		}(part)
	}
}

func (p *Pipeline) Stop() {
	p.cancel()
	p.wg.Wait()
	for _, part := range p.partitions {
		part.Stop()
	}
}
