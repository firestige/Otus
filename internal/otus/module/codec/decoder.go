package codec

import (
	"context"

	"github.com/google/gopacket"
)

// SimplifiedDecoder 简化解码器
type SimplifiedDecoder struct {
	processor PacketProcessor
}

// NewSimplifiedDecoder 创建简化解码器
func NewSimplifiedDecoder(outputChan chan<- *NetworkMessage) (*SimplifiedDecoder, error) {
	processor, err := NewIPv4PacketProcessor(DefaultProcessorConfig(), outputChan)
	if err != nil {
		return nil, err
	}

	return &SimplifiedDecoder{
		processor: processor,
	}, nil
}

// Process 处理网络包数据 - 保持接口不变
func (d *SimplifiedDecoder) Process(data []byte, ci *gopacket.CaptureInfo) {
	if d.processor != nil {
		if ipv4Processor, ok := d.processor.(*IPv4PacketProcessor); ok {
			ipv4Processor.Process(data, ci)
		}
	}
}

// Start 启动解码器
func (d *SimplifiedDecoder) Start() error {
	if d.processor != nil {
		return d.processor.Start(context.Background())
	}
	return nil
}

// Stop 停止解码器
func (d *SimplifiedDecoder) Stop() error {
	if d.processor != nil {
		return d.processor.Stop()
	}
	return nil
}
