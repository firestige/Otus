package sniffer

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/gopacket"
)

var (
	factory CaptureHandleFactory
	once    *sync.Once
)

func HandleFactory() *CaptureHandleFactory {
	once.Do(func() {
		factory = *NewCaptureHandleFactory()
	})
	return &factory
}

// CaptureHandle 定义抓包句柄接口
type CaptureHandle interface {
	// Open 打开抓包句柄
	Open(options *Options) error

	// ReadPacket 读取数据包
	ReadPacket() ([]byte, gopacket.CaptureInfo, error)

	// Close 关闭抓包句柄
	Close() error

	// GetType 获取抓包类型
	GetType() CaptureType
}

// CaptureOptions 抓包选项配置
type CaptureOptions struct {
	SnapLen     int    // 捕获长度
	BufferSize  int    // 缓冲区大小
	SupportVlan bool   // 是否支持 VLAN
	Timeout     int    // 超时时间 (毫秒)
	Filter      string // BPF 过滤器
	FanoutId    uint16 // Fanout ID (可选)
}

// DefaultCaptureOptions 返回默认的抓包选项
func DefaultCaptureOptions() *Options {
	return &Options{
		BufferSize: 1024 * 1024, // 1MB
		Timeout:    1000,        // 1秒
		SnapLen:    65536,
		Filter:     "",
	}
}

// CaptureHandleFactory 抓包句柄工厂
type CaptureHandleFactory struct{}

// NewCaptureHandleFactory 创建新的工厂实例
func NewCaptureHandleFactory() *CaptureHandleFactory {
	return &CaptureHandleFactory{}
}

// CreateHandle 根据类型创建抓包句柄
func (f *CaptureHandleFactory) CreateHandle(ctx context.Context, captureType CaptureType) (CaptureHandle, error) {
	switch captureType {
	case TypeAFPacket:
		return NewAFPacketHandle(ctx), nil
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
