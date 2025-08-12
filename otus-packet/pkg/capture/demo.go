package capture

import (
	"fmt"
	"time"
)

// DemoFactoryUsage 展示工厂模式的使用示例
func DemoFactoryUsage() {
	fmt.Println("=== Capture Factory Pattern Demo ===")

	// 1. 创建工厂
	factory := NewCaptureHandleFactory()

	// 2. 查看支持的类型
	fmt.Printf("Supported types: %v\n", factory.GetSupportedTypes())

	// 3. 创建 AF_PACKET 句柄
	handle, err := factory.CreateHandle(TypeAFPacket)
	if err != nil {
		fmt.Printf("Error creating handle: %v\n", err)
		return
	}

	fmt.Printf("Created handle of type: %s\n", handle.GetType())

	// 4. 使用抓包管理器
	manager := NewCaptureManager()

	// 配置选项
	options := &CaptureOptions{
		BufferSize:   1024 * 1024, // 1MB
		Promiscuous:  true,
		Timeout:      1000, // 1s
		SnapLen:      1500,
		Filter:       "",
		BlockingMode: false,
	}

	// 尝试启动 (注意: 在测试环境中可能会失败，这是正常的)
	err = manager.Start(TypeAFPacket, "lo", options)
	if err != nil {
		fmt.Printf("Note: Start failed (expected in test env): %v\n", err)
	} else {
		fmt.Println("Capture started successfully")

		// 获取统计信息
		stats, err := manager.GetStats()
		if err != nil {
			fmt.Printf("Error getting stats: %v\n", err)
		} else {
			fmt.Printf("Initial stats: %+v\n", stats)
		}

		// 停止抓包
		manager.Stop()
		fmt.Println("Capture stopped")
	}

	// 5. 使用高级 netCapture 接口
	nc := GetInstance()
	fmt.Printf("NetCapture supported types: %v\n", nc.GetSupportedCaptureTypes())

	// 尝试使用自定义配置启动
	err = nc.StartWithConfig(TypeAFPacket, "lo", options)
	if err != nil {
		fmt.Printf("Note: NetCapture start failed (expected): %v\n", err)
	} else {
		fmt.Printf("NetCapture started with type: %s\n", nc.GetCurrentCaptureType())

		// 等待一小段时间
		time.Sleep(100 * time.Millisecond)

		// 停止
		nc.Stop()
		fmt.Println("NetCapture stopped")
	}

	fmt.Println("=== Demo Complete ===")
}
