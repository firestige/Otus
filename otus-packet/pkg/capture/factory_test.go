package capture

import (
	"fmt"
	"testing"
)

// TestCaptureFactory 测试抓包工厂模式
func TestCaptureFactory(t *testing.T) {
	// 创建工厂
	factory := NewCaptureHandleFactory()

	// 测试获取支持的类型
	supportedTypes := factory.GetSupportedTypes()
	fmt.Printf("Supported capture types: %v\n", supportedTypes)

	// 测试类型支持检查
	if !factory.IsTypeSupported(TypeAFPacket) {
		t.Errorf("AF_PACKET should be supported")
	}

	if factory.IsTypeSupported(TypePCAP) {
		t.Errorf("PCAP should not be supported yet")
	}

	// 测试创建 AF_PACKET 句柄
	handle, err := factory.CreateHandle(TypeAFPacket)
	if err != nil {
		t.Errorf("Failed to create AF_PACKET handle: %v", err)
	}

	if handle.GetType() != TypeAFPacket {
		t.Errorf("Expected AF_PACKET type, got %s", handle.GetType())
	}

	// 测试创建不支持的类型
	_, err = factory.CreateHandle(TypePCAP)
	if err == nil {
		t.Errorf("Should fail to create PCAP handle")
	}
}

// TestCaptureManager 测试抓包管理器
func TestCaptureManager(t *testing.T) {
	manager := NewCaptureManager()

	// 测试获取支持的类型
	types := manager.GetSupportedTypes()
	if len(types) == 0 {
		t.Errorf("Should have at least one supported type")
	}

	// 测试活跃状态检查
	if manager.IsActive() {
		t.Errorf("Manager should not be active initially")
	}

	// 测试启动不支持的类型
	err := manager.Start(TypePCAP, "lo", nil)
	if err == nil {
		t.Errorf("Should fail to start unsupported capture type")
	}
}

// TestNetCapture 测试网络抓包单例
func TestNetCapture(t *testing.T) {
	nc := GetInstance()

	// 测试获取支持的类型
	types := nc.GetSupportedCaptureTypes()
	fmt.Printf("NetCapture supported types: %v\n", types)

	// 测试初始状态
	if nc.IsRunning() {
		t.Errorf("NetCapture should not be running initially")
	}

	// 测试获取当前类型 (应该为空)
	currentType := nc.GetCurrentCaptureType()
	if currentType != "" {
		t.Errorf("Current capture type should be empty when not running")
	}
}
