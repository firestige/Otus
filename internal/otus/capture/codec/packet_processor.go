package codec

import (
	"context"
	"net"
	"time"
)

// PacketProcessor 网络包处理器主接口
type PacketProcessor interface {
	ProcessPacket(ctx context.Context, rawData []byte, meta *CaptureMetadata) error
}

// CaptureMetadata 捕获包的元数据
type CaptureMetadata struct {
	Timestamp      time.Time
	CaptureLength  int
	PacketLength   int
	InterfaceIndex int
}

// NetworkMessage 解析后的网络消息
type NetworkMessage struct {
	IPVersion       byte
	TransportProto  byte
	SourceAddr      net.IP
	DestinationAddr net.IP
	SourcePort      uint16
	DestinationPort uint16
	TimestampSec    uint32
	TimestampMicro  uint32
	ApplicationType byte
	Content         []byte
	CallID          []byte
	TCPFlags        uint8
}

// ProtocolHandler 协议处理器接口
type ProtocolHandler interface {
	CanHandle(transportProto byte, port uint16, payload []byte) bool
	Handle(msg *NetworkMessage) (*ProtocolResult, error)
	GetType() byte
}

// ProtocolResult 协议处理结果
type ProtocolResult struct {
	MessageType      byte
	ProcessedContent []byte
	SessionID        []byte
}

// ProcessorConfig 处理器配置
type ProcessorConfig struct {
	EnableTCPReassembly bool
	OutputChannelSize   int
	MetricsInterval     time.Duration
	FragmentTimeout     time.Duration
}

// DefaultProcessorConfig 默认配置
func DefaultProcessorConfig() *ProcessorConfig {
	return &ProcessorConfig{
		EnableTCPReassembly: true,
		OutputChannelSize:   20000,
		MetricsInterval:     time.Minute,
		FragmentTimeout:     30 * time.Second,
	}
}
