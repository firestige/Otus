package capture

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// CaptureStats 抓包统计信息
type CaptureStats struct {
	// Info 部分
	NetworkInterface string
	ListenIP         string
	ListenPort       int
	Protocol         string
	StartTime        time.Time

	// 抓包执行统计 (使用 atomic 保证并发安全)
	TotalPackets   int64
	DroppedPackets int64
	TCPPackets     int64
	UDPPackets     int64
	ICMPPackets    int64
	OtherPackets   int64

	// 速度统计
	lastTotalPackets   int64
	lastDroppedPackets int64
	lastUpdateTime     time.Time
	inboundRate        int64 // 包/秒
	outboundRate       int64 // 包/秒

	// 传输队列统计
	queueLength   int64
	queueCapacity int64
	queueInRate   int64 // 入队速度 包/秒
	queueOutRate  int64 // 出队速度 包/秒

	mu sync.RWMutex
}

// NewCaptureStats 创建新的统计实例
func NewCaptureStats(networkInterface, listenIP string, listenPort int, protocol string) *CaptureStats {
	return &CaptureStats{
		NetworkInterface: networkInterface,
		ListenIP:         listenIP,
		ListenPort:       listenPort,
		Protocol:         protocol,
		StartTime:        time.Now(),
		lastUpdateTime:   time.Now(),
		queueCapacity:    10000, // 默认队列容量
	}
}

// UpdatePacketStats 更新包统计信息
func (cs *CaptureStats) UpdatePacketStats(packetType string, isDropped bool) {
	atomic.AddInt64(&cs.TotalPackets, 1)

	if isDropped {
		atomic.AddInt64(&cs.DroppedPackets, 1)
		return
	}

	switch packetType {
	case "TCP":
		atomic.AddInt64(&cs.TCPPackets, 1)
	case "UDP":
		atomic.AddInt64(&cs.UDPPackets, 1)
	case "ICMP":
		atomic.AddInt64(&cs.ICMPPackets, 1)
	default:
		atomic.AddInt64(&cs.OtherPackets, 1)
	}
}

// UpdateQueueStats 更新队列统计信息
func (cs *CaptureStats) UpdateQueueStats(currentLength, capacity int64) {
	atomic.StoreInt64(&cs.queueLength, currentLength)
	atomic.StoreInt64(&cs.queueCapacity, capacity)
}

// UpdateRates 更新速度统计
func (cs *CaptureStats) UpdateRates() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(cs.lastUpdateTime).Seconds()

	if elapsed > 0 {
		currentTotal := atomic.LoadInt64(&cs.TotalPackets)
		currentDropped := atomic.LoadInt64(&cs.DroppedPackets)

		// 计算速度
		totalDiff := currentTotal - cs.lastTotalPackets
		droppedDiff := currentDropped - cs.lastDroppedPackets

		cs.inboundRate = int64(float64(totalDiff) / elapsed)
		cs.outboundRate = int64(float64(totalDiff-droppedDiff) / elapsed)

		// 更新上次记录
		cs.lastTotalPackets = currentTotal
		cs.lastDroppedPackets = currentDropped
		cs.lastUpdateTime = now
	}
}

// GetDropRate 获取丢包率
func (cs *CaptureStats) GetDropRate() float64 {
	total := atomic.LoadInt64(&cs.TotalPackets)
	dropped := atomic.LoadInt64(&cs.DroppedPackets)

	if total == 0 {
		return 0.0
	}
	return float64(dropped) / float64(total) * 100
}

// GetCPUUsage 获取CPU使用率 (简化版)
func (cs *CaptureStats) GetCPUUsage() float64 {
	// 这里可以集成更复杂的CPU监控逻辑
	return float64(runtime.NumGoroutine()) * 0.1 // 简化示例
}

// GetMemoryUsage 获取内存使用情况
func (cs *CaptureStats) GetMemoryUsage() (float64, float64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	allocMB := float64(m.Alloc) / 1024 / 1024
	sysMB := float64(m.Sys) / 1024 / 1024

	return allocMB, sysMB
}

// GetRuntime 获取运行时间
func (cs *CaptureStats) GetRuntime() time.Duration {
	return time.Since(cs.StartTime)
}

// PrintStats 打印统计信息到控制台
func (cs *CaptureStats) PrintStats(showHeader bool) {
	if showHeader {
		fmt.Println("==================================================")
		fmt.Println("                OTUS Packet Capture Status        ")
		fmt.Println("==================================================")
	}

	// Info 部分
	if showHeader {
		fmt.Println("\n[CAPTURE INFO]")
	} else {
		fmt.Println("[CAPTURE INFO]")
	}
	fmt.Printf("  Network Interface: %s\n", cs.NetworkInterface)
	fmt.Printf("  Listen IP:         %s\n", cs.ListenIP)
	fmt.Printf("  Listen Port:       %d\n", cs.ListenPort)
	fmt.Printf("  Protocol Filter:   %s\n", cs.Protocol)
	fmt.Printf("  Runtime:           %v\n", cs.GetRuntime().Truncate(time.Second))

	allocMB, sysMB := cs.GetMemoryUsage()
	fmt.Printf("  CPU Usage:         %.2f%%\n", cs.GetCPUUsage())
	fmt.Printf("  Memory Usage:      %.2f MB (Alloc) / %.2f MB (Sys)\n", allocMB, sysMB)

	// 抓包执行统计
	fmt.Println("\n[PACKET CAPTURE STATS]")
	totalPackets := atomic.LoadInt64(&cs.TotalPackets)
	droppedPackets := atomic.LoadInt64(&cs.DroppedPackets)

	fmt.Printf("  Total Packets:     %d\n", totalPackets)
	fmt.Printf("  Dropped Packets:   %d\n", droppedPackets)
	fmt.Printf("  Drop Rate:         %.2f%%\n", cs.GetDropRate())
	fmt.Printf("  TCP Packets:       %d\n", atomic.LoadInt64(&cs.TCPPackets))
	fmt.Printf("  UDP Packets:       %d\n", atomic.LoadInt64(&cs.UDPPackets))
	fmt.Printf("  ICMP Packets:      %d\n", atomic.LoadInt64(&cs.ICMPPackets))
	fmt.Printf("  Other Packets:     %d\n", atomic.LoadInt64(&cs.OtherPackets))

	cs.mu.RLock()
	fmt.Printf("  Inbound Rate:      %d pps\n", cs.inboundRate)
	fmt.Printf("  Outbound Rate:     %d pps\n", cs.outboundRate)
	cs.mu.RUnlock()

	// 传输执行统计
	fmt.Println("\n[QUEUE STATS]")
	queueLength := atomic.LoadInt64(&cs.queueLength)
	queueCapacity := atomic.LoadInt64(&cs.queueCapacity)
	queueAvailable := queueCapacity - queueLength

	fmt.Printf("  Queue Length:      %d\n", queueLength)
	fmt.Printf("  Queue Capacity:    %d\n", queueCapacity)
	fmt.Printf("  Available Space:   %d\n", queueAvailable)
	fmt.Printf("  Queue Usage:       %.2f%%\n", float64(queueLength)/float64(queueCapacity)*100)
	fmt.Printf("  Queue In Rate:     %d pps\n", atomic.LoadInt64(&cs.queueInRate))
	fmt.Printf("  Queue Out Rate:    %d pps\n", atomic.LoadInt64(&cs.queueOutRate))

	if showHeader {
		fmt.Println("\n==================================================")
	} else {
		fmt.Println() // 在 headless 模式下只添加一个空行
	}
}

// StartStatsMonitor 启动统计监控 goroutine
func (cs *CaptureStats) StartStatsMonitor(interval time.Duration, ctx context.Context) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				cs.UpdateRates()
			case <-ctx.Done():
				return
			}
		}
	}()
}
