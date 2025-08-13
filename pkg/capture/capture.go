package capture

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type netCapture struct {
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	stats     *CaptureStats
	isRunning bool
	manager   *CaptureManager
}

var (
	instance *netCapture
	once     sync.Once
)

// GetInstance 返回 netCapture 的单例实例
func GetInstance() *netCapture {
	once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		instance = &netCapture{
			ctx:       ctx,
			cancel:    cancel,
			isRunning: false,
			manager:   NewCaptureManager(),
		}
	})
	return instance
}

func (nc *netCapture) Start() error {
	// 使用默认配置启动
	return nc.StartWithConfig(TypeAFPacket, "eth0", DefaultCaptureOptions())
}

// StartWithConfig 使用指定配置启动抓包
func (nc *netCapture) StartWithConfig(captureType CaptureType, interfaceName string, options *CaptureOptions) error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if nc.isRunning {
		return nil // 已经在运行
	}

	// 启动抓包管理器
	err := nc.manager.Start(captureType, interfaceName, options)
	if err != nil {
		return fmt.Errorf("failed to start capture manager: %v", err)
	}

	// 初始化统计信息
	nc.stats = NewCaptureStats(interfaceName, "0.0.0.0", 0, "all")
	nc.isRunning = true

	// 启动统计监控
	nc.stats.StartStatsMonitor(1*time.Second, nc.ctx)

	// 启动网络捕获逻辑
	go func() {
		defer func() {
			nc.mu.Lock()
			nc.isRunning = false
			if nc.manager != nil {
				nc.manager.Stop()
			}
			nc.mu.Unlock()
		}()

		// 主抓包循环
		for {
			select {
			case <-nc.ctx.Done():
				// 收到取消信号，优雅停止
				return
			default:
				// 读取数据包
				data, _, err := nc.manager.ReadPacket()
				if err != nil {
					// 处理读取错误，可能是超时等正常情况
					continue
				}

				// 更新统计信息
				if nc.stats != nil {
					// 这里可以解析包类型，暂时使用默认值
					nc.stats.UpdatePacketStats("TCP", false)
				}

				// 处理数据包 (这里可以添加实际的包处理逻辑)
				_ = data
			}
		}
	}()

	return nil
}

func (nc *netCapture) Stop() error {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if !nc.isRunning {
		return nil // 已经停止
	}

	// 停止抓包管理器
	if nc.manager != nil {
		err := nc.manager.Stop()
		if err != nil {
			fmt.Printf("Warning: failed to stop capture manager: %v\n", err)
		}
	}

	if nc.cancel != nil {
		nc.cancel()
	}
	nc.isRunning = false
	return nil
}

func (nc *netCapture) Status(refresh bool, interval time.Duration, headless bool) {
	nc.mu.Lock()
	stats := nc.stats
	running := nc.isRunning
	nc.mu.Unlock()

	if !running || stats == nil {
		fmt.Println("Capture is not running")
		return
	}

	if refresh {
		// 持续刷新模式
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// 首次显示
		if !headless {
			fmt.Print("\033[2J\033[H") // 清屏
		}
		stats.PrintStats(!headless)

		for {
			select {
			case <-ticker.C:
				if !headless {
					// 清屏 (非 headless 模式)
					fmt.Print("\033[2J\033[H")
				}
				stats.PrintStats(!headless)
			case <-ctx.Done():
				return
			}
		}
	} else {
		// 只显示当前状态
		stats.PrintStats(!headless)
	}
}

// GetStats 获取统计实例 (用于外部更新统计数据)
func (nc *netCapture) GetStats() *CaptureStats {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	return nc.stats
}

// IsRunning 检查是否正在运行
func (nc *netCapture) IsRunning() bool {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	return nc.isRunning
}

// GetSupportedCaptureTypes 获取支持的抓包类型
func (nc *netCapture) GetSupportedCaptureTypes() []CaptureType {
	return nc.manager.GetSupportedTypes()
}

// GetCurrentCaptureType 获取当前使用的抓包类型
func (nc *netCapture) GetCurrentCaptureType() CaptureType {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if !nc.isRunning || nc.manager == nil {
		return ""
	}
	return nc.manager.GetCurrentType()
}

// GetCaptureManager 获取抓包管理器 (用于高级操作)
func (nc *netCapture) GetCaptureManager() *CaptureManager {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	return nc.manager
}
