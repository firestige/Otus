package pipeline

import (
	"context"
	"fmt"

	otus "firestige.xyz/otus/internal/otus/api"
	module "firestige.xyz/otus/internal/otus/module/api"
	"github.com/google/gopacket"
)

type partition struct {
	id   int
	name string

	dataSource    module.DataSource
	packetHandler *packetHandler
	senders       []module.Sender
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
	p.dataSource.Boot()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 处理数据包的逻辑
			data, info, err := p.dataSource.ReadPacket()
			if err != nil {
				// 处理错误
				continue
			}
			// 处理数据包
			exchange := p.buildExchange(data, info)
			p.packetHandler.handle(exchange)
			for _, sender := range p.senders {
				sender.Send(exchange)
			}
		}
	}
}

func (p *partition) Stop() {
	// 停止数据源和发送器
	p.dataSource.Stop()
	for _, sender := range p.senders {
		sender.Shutdown()
	}
}

func (p *partition) buildExchange(data []byte, ci gopacket.CaptureInfo) *otus.Exchange {
	// Todo: 构建 Exchange 对象的逻辑
	return &otus.Exchange{}
}
