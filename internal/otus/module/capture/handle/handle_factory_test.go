package handle

import (
	"fmt"
	"testing"
)

// TestCaptureFactory 测试抓包工厂模式
func TestCaptureFactory(t *testing.T) {
	// 创建工厂
	factory := newCaptureHandleFactory()

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
	options := &Options{
		NetworkInterface: "eth0",
		SnapLen:          1600,
		BufferSize:       1024 * 1024,
		Timeout:          1000,
		Filter:           "udp port 5060",
		CaptureType:      TypeAFPacket,
	}

	handle, err := factory.CreateHandle(options)
	if err != nil {
		t.Errorf("Failed to create AF_PACKET handle: %v", err)
	}

	if handle.GetType() != TypeAFPacket {
		t.Errorf("Expected AF_PACKET type, got %s", handle.GetType())
	}

	// 测试创建不支持的类型
	pcapOptions := &Options{
		NetworkInterface: "eth0",
		CaptureType:      TypePCAP,
	}
	_, err = factory.CreateHandle(pcapOptions)
	if err == nil {
		t.Errorf("Should fail to create PCAP handle")
	}
}
