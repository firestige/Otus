package pipeline

import (
	"context"
	"syscall"
	"time"

	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/pcap"
)

type PacketData interface {
}

type CaptureInfo interface {
	GetTimestamp() time.Time
	GetLenth() int
	GetCaptureLength() int
	IsTruncated() bool
}

type DataSource interface {
	ReadPacketData() (PacketData, CaptureInfo, error)
	ZeroCopyReadPacketData() (PacketData, CaptureInfo, error)
}

type Decoder interface {
	Decode(data PacketData, ci CaptureInfo) (interface{}, error)
}

type Exchange struct {
	msg interface{}
	ci  CaptureInfo
}

type Filter interface {
	Filter(exchange *Exchange, chain FilterChain) error
}

type FilterChain interface {
	Filter(exchange *Exchange) error
}

type Dispatcher interface {
	Dispatch(exchange *Exchange) error
}

type Sender interface {
	Support(exchange *Exchange) bool
	Channel() chan<- *Exchange
}

type pipeline struct {
	ds          DataSource
	decoder     Decoder
	filterChain FilterChain
	dispatcher  Dispatcher
	sender      Sender

	ctx    context.Context
	cancel context.CancelFunc
}

func (p *pipeline) run() {
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
			// Main processing logic
			packet, ci, err := p.ds.ReadPacketData()
			if err == pcap.NextErrorTimeoutExpired || err == afpacket.ErrTimeout || err == syscall.EINTR {
				continue
			}
			if err != nil {
				// Handle error, possibly log it
				break
			}
			// 处理数据包
			p.OnPacket(packet, ci)
		}
	}
}

func (p *pipeline) OnPacket(packet PacketData, ci CaptureInfo) {
	// 处理接收到的数据包
	msg, err := p.decoder.Decode(packet, ci)
	if err != nil {
		// 处理解码错误
		return
	}
	exchange := &Exchange{
		msg: msg,
		ci:  ci,
	}
	err = p.filterChain.Filter(exchange)
	if err != nil {
		return
	}
}

func (p *pipeline) Start() error {
	// 组装上下文
	p.ctx, p.cancel = context.WithCancel(context.Background())
	// 启动sender作为最终的消费者
	// go p.Sender.Send()
	// 启动流水线的处理逻辑，向channel中生产数据
	// go p.run()
	return nil
}

func (p *pipeline) Stop() error {
	// 发送停止信号
	p.cancel()
	// 等待go程停止
	// 回收资源
	return nil
}
