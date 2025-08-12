package capture

import (
	"fmt"

	"github.com/google/gopacket"
)

// CaptureType 定义抓包类型
type CaptureType string

const (
	TypeAFPacket CaptureType = "afpacket"
	TypePCAP     CaptureType = "pcap"
	TypeXDP      CaptureType = "xdp"
)

// CaptureHandle 定义抓包句柄接口
type CaptureHandle interface {
	// Open 打开抓包句柄
	Open(interfaceName string, options *CaptureOptions) error

	// ReadPacket 读取数据包
	ReadPacket() ([]byte, gopacket.CaptureInfo, error)

	// Close 关闭抓包句柄
	Close() error

	// GetStats 获取抓包统计信息
	GetStats() (*HandleStats, error)

	// GetType 获取抓包类型
	GetType() CaptureType
}

// CaptureOptions 抓包选项配置
type CaptureOptions struct {
	BufferSize   int    // 缓冲区大小
	Promiscuous  bool   // 混杂模式
	Timeout      int    // 超时时间 (毫秒)
	SnapLen      int    // 捕获长度
	Filter       string // BPF 过滤器
	BlockingMode bool   // 阻塞模式
	FanoutId     uint16 // Fanout ID (可选)
}

// HandleStats 抓包句柄统计信息
type HandleStats struct {
	PacketsReceived uint64
	PacketsDropped  uint64
	PacketsSent     uint64
	Errors          uint64
}

// DefaultCaptureOptions 返回默认的抓包选项
func DefaultCaptureOptions() *CaptureOptions {
	return &CaptureOptions{
		BufferSize:   1024 * 1024, // 1MB
		Promiscuous:  true,
		Timeout:      1000, // 1秒
		SnapLen:      65536,
		Filter:       "",
		BlockingMode: false,
	}
}

// CaptureHandleFactory 抓包句柄工厂
type CaptureHandleFactory struct{}

// NewCaptureHandleFactory 创建新的工厂实例
func NewCaptureHandleFactory() *CaptureHandleFactory {
	return &CaptureHandleFactory{}
}

// CreateHandle 根据类型创建抓包句柄
func (f *CaptureHandleFactory) CreateHandle(captureType CaptureType) (CaptureHandle, error) {
	switch captureType {
	case TypeAFPacket:
		return NewAFPacketHandle(), nil
	case TypePCAP:
		return nil, fmt.Errorf("PCAP capture type not implemented yet")
	case TypeXDP:
		return nil, fmt.Errorf("XDP capture type not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported capture type: %s", captureType)
	}
}

// GetSupportedTypes 获取支持的抓包类型列表
func (f *CaptureHandleFactory) GetSupportedTypes() []CaptureType {
	return []CaptureType{
		TypeAFPacket,
		// TypePCAP,    // 未实现
		// TypeXDP,     // 未实现
	}
}

// IsTypeSupported 检查指定类型是否支持
func (f *CaptureHandleFactory) IsTypeSupported(captureType CaptureType) bool {
	supportedTypes := f.GetSupportedTypes()
	for _, t := range supportedTypes {
		if t == captureType {
			return true
		}
	}
	return false
}
