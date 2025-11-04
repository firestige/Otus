package pipeline

import (
	"context"
	"sync"

	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/otus/factory"
)

type PipelineConfig struct {
	Name           string
	PartitionCount int
	SourceCfg      *config.SourceConfig
	DecoderCfg     *config.DecoderConfig
	FiltersCfg     *config.FiltersConfig
	SinksCfgs      *config.SinksConfig
}

type Pipeline struct {
	cfg *PipelineConfig

	partitions []*partition

	ctx    context.Context
	cancel context.CancelFunc
	wg     *sync.WaitGroup
}

func NewPipeline(cfg *PipelineConfig) *Pipeline {
	return &Pipeline{
		cfg:        cfg,
		partitions: make([]*partition, 0),
		wg:         &sync.WaitGroup{},
	}
}

func (p *Pipeline) Name() string {
	return p.cfg.Name
}

func (p *Pipeline) Init() {
	for i := 0; i < p.cfg.PartitionCount; i++ {
		part := newPartition(i, p)
		part.source = factory.GetSource(p.cfg.SourceCfg)
		part.decoder = factory.GetDecoder(p.cfg.DecoderCfg)
		filters := factory.GetFilters(p.cfg.FiltersCfg)
		sinks := factory.GetSinks(p.cfg.SinksCfgs)
		part.processor = factory.GetProcessor(filters, sinks)
		p.partitions = append(p.partitions, part)
	}

}

func (p *Pipeline) Start(ctx context.Context) {
	p.ctx, p.cancel = context.WithCancel(ctx)
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
