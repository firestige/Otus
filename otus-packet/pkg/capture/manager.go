package capture

import (
	"fmt"
	"log"

	"github.com/google/gopacket"
)

// CaptureManager 抓包管理器，演示工厂模式的使用
type CaptureManager struct {
	factory *CaptureHandleFactory
	handle  CaptureHandle
}

// NewCaptureManager 创建新的抓包管理器
func NewCaptureManager() *CaptureManager {
	return &CaptureManager{
		factory: NewCaptureHandleFactory(),
	}
}

// Start 启动抓包
func (cm *CaptureManager) Start(captureType CaptureType, interfaceName string, options *CaptureOptions) error {
	// 检查类型是否支持
	if !cm.factory.IsTypeSupported(captureType) {
		return fmt.Errorf("capture type %s is not supported", captureType)
	}

	// 创建抓包句柄
	handle, err := cm.factory.CreateHandle(captureType)
	if err != nil {
		return fmt.Errorf("failed to create capture handle: %v", err)
	}

	// 打开抓包句柄
	if options == nil {
		options = DefaultCaptureOptions()
	}

	err = handle.Open(interfaceName, options)
	if err != nil {
		return fmt.Errorf("failed to open capture handle: %v", err)
	}

	cm.handle = handle
	log.Printf("Started %s capture on interface %s", captureType, interfaceName)
	return nil
}

// Stop 停止抓包
func (cm *CaptureManager) Stop() error {
	if cm.handle == nil {
		return fmt.Errorf("no active capture handle")
	}

	err := cm.handle.Close()
	if err != nil {
		return fmt.Errorf("failed to close capture handle: %v", err)
	}

	log.Printf("Stopped %s capture", cm.handle.GetType())
	cm.handle = nil
	return nil
}

// ReadPacket 读取数据包
func (cm *CaptureManager) ReadPacket() (data []byte, ci gopacket.CaptureInfo, err error) {
	if cm.handle == nil {
		return nil, ci, fmt.Errorf("no active capture handle")
	}

	return cm.handle.ReadPacket()
}

// GetStats 获取统计信息
func (cm *CaptureManager) GetStats() (*HandleStats, error) {
	if cm.handle == nil {
		return nil, fmt.Errorf("no active capture handle")
	}

	return cm.handle.GetStats()
}

// GetSupportedTypes 获取支持的抓包类型
func (cm *CaptureManager) GetSupportedTypes() []CaptureType {
	return cm.factory.GetSupportedTypes()
}

// GetCurrentType 获取当前使用的抓包类型
func (cm *CaptureManager) GetCurrentType() CaptureType {
	if cm.handle == nil {
		return ""
	}
	return cm.handle.GetType()
}

// IsActive 检查是否有活跃的抓包会话
func (cm *CaptureManager) IsActive() bool {
	return cm.handle != nil
}

// Example 展示如何使用抓包管理器
func ExampleUsage() {
	// 创建抓包管理器
	manager := NewCaptureManager()

	// 查看支持的类型
	fmt.Println("Supported capture types:", manager.GetSupportedTypes())

	// 配置抓包选项
	options := &CaptureOptions{
		BufferSize: 2 * 1024 * 1024, // 2MB
		Timeout:    500,             // 500ms
		SnapLen:    1500,
		Filter:     "tcp port 80",
	}

	// 启动 AF_PACKET 抓包
	err := manager.Start(TypeAFPacket, "eth0", options)
	if err != nil {
		log.Printf("Failed to start capture: %v", err)
		return
	}

	// 读取数据包
	for i := 0; i < 10; i++ {
		data, _, err := manager.ReadPacket()
		if err != nil {
			log.Printf("Error reading packet: %v", err)
			continue
		}
		fmt.Printf("Received packet of length %d\n", len(data))
	}

	// 获取统计信息
	stats, err := manager.GetStats()
	if err != nil {
		log.Printf("Error getting stats: %v", err)
	} else {
		fmt.Printf("Stats: Received=%d, Dropped=%d, Sent=%d, Errors=%d\n",
			stats.PacketsReceived, stats.PacketsDropped, stats.PacketsSent, stats.Errors)
	}

	// 停止抓包
	err = manager.Stop()
	if err != nil {
		log.Printf("Failed to stop capture: %v", err)
	}
}
