package pipeline

import (
	"context"
	"fmt"

	otus "firestige.xyz/otus/internal/otus/api"
)

type partition struct {
	id   int
	name string

	source    otus.Source
	decoder   otus.Decoder
	processor otus.Processor
	sinks     []otus.Sink
}

func newPartition(id int, p *Pipeline) *partition {
	return &partition{
		id:   id,
		name: fmt.Sprintf("%s-partition-%d", p.Name(), id),
	}
}

func (p *partition) ID() int {
	return p.id
}

func (p *partition) Name() string {
	return p.name
}

func (p *partition) String() string {
	return p.name
}

func (p *partition) Start(ctx context.Context) {
	p.source.Start(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 处理数据包的逻辑
			data, info, err := p.source.ReadPacket()
			if err != nil {
				// 处理错误
				continue
			}
			// 处理数据包
			packet, err := p.decoder.Decode(data, info)

			if err != nil {
				// 处理解码错误
				continue
			}

			exchange := &otus.Exchange{
				Packet:  packet,
				Context: make(map[string]interface{}),
			}
			p.processor.Process(exchange)
			for _, sink := range p.sinks {
				sink.Send(exchange)
			}
		}
	}
}

func (p *partition) Stop() {
	// 停止数据源和发送器
	p.source.Stop()
	for _, sender := range p.sinks {
		sender.Close()
	}
}
