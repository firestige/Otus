package plugin

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/pkg/plugin"
)

// TestMain runs before all tests in this package
func TestMain(m *testing.M) {
	// 初始化日志框架
	log.Init(&log.LoggerConfig{
		Level:   "info",
		Pattern: "%time[%caller][%func][%goroutine][%level][%field] - %msg\n",
		Time:    "2006-01-02 15:04:05",
	})

	// 运行所有测试
	code := m.Run()

	// 清理资源（如果需要）
	// cleanup()

	os.Exit(code)
}

// MockPlugin 模拟插件（用于测试）
type MockPlugin struct {
	meta         plugin.Metadata
	initCalled   atomic.Bool
	startCalled  atomic.Bool
	stopCalled   atomic.Bool
	healthCalled atomic.Bool

	// 控制行为
	initDelay   time.Duration
	startDelay  time.Duration
	stopDelay   time.Duration
	healthDelay time.Duration

	initError   error
	startError  error
	stopError   error
	healthError error
}

// NewMockPlugin 创建模拟插件
func NewMockPlugin(name, pluginType string, deps []string) *MockPlugin {
	return &MockPlugin{
		meta: plugin.Metadata{
			Name:         name,
			Type:         pluginType,
			Version:      "1.0.0",
			Description:  fmt.Sprintf("Mock %s plugin", name),
			Dependencies: deps,
		},
	}
}

// Metadata 返回元数据
func (m *MockPlugin) Metadata() plugin.Metadata {
	return m.meta
}

// Init 初始化
func (m *MockPlugin) Init(config map[string]interface{}) error {
	if m.initDelay > 0 {
		time.Sleep(m.initDelay)
	}
	m.initCalled.Store(true)
	return m.initError
}

// Start 启动
func (m *MockPlugin) Start() error {
	if m.startDelay > 0 {
		time.Sleep(m.startDelay)
	}
	m.startCalled.Store(true)
	return m.startError
}

// Stop 停止
func (m *MockPlugin) Stop() error {
	if m.stopDelay > 0 {
		time.Sleep(m.stopDelay)
	}
	m.stopCalled.Store(true)
	return m.stopError
}

// Health 健康检查
func (m *MockPlugin) Health() error {
	if m.healthDelay > 0 {
		time.Sleep(m.healthDelay)
	}
	m.healthCalled.Store(true)
	return m.healthError
}

// 测试辅助方法
func (m *MockPlugin) WasInitCalled() bool   { return m.initCalled.Load() }
func (m *MockPlugin) WasStartCalled() bool  { return m.startCalled.Load() }
func (m *MockPlugin) WasStopCalled() bool   { return m.stopCalled.Load() }
func (m *MockPlugin) WasHealthCalled() bool { return m.healthCalled.Load() }

// SetInitDelay 设置初始化延迟
func (m *MockPlugin) SetInitDelay(d time.Duration)   { m.initDelay = d }
func (m *MockPlugin) SetStartDelay(d time.Duration)  { m.startDelay = d }
func (m *MockPlugin) SetStopDelay(d time.Duration)   { m.stopDelay = d }
func (m *MockPlugin) SetHealthDelay(d time.Duration) { m.healthDelay = d }

// SetInitError 设置初始化错误
func (m *MockPlugin) SetInitError(err error)   { m.initError = err }
func (m *MockPlugin) SetStartError(err error)  { m.startError = err }
func (m *MockPlugin) SetStopError(err error)   { m.stopError = err }
func (m *MockPlugin) SetHealthError(err error) { m.healthError = err }

// Reset 重置状态
func (m *MockPlugin) Reset() {
	m.initCalled.Store(false)
	m.startCalled.Store(false)
	m.stopCalled.Store(false)
	m.healthCalled.Store(false)
}
