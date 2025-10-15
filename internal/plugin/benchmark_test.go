package plugin

import (
	"fmt"
	"testing"
	"time"

	"firestige.xyz/otus/pkg/plugin"
)

func BenchmarkRegistry_Register(b *testing.B) {
	r := NewRegistry()
	ps := make([]plugin.Plugin, b.N)

	for i := 0; i < b.N; i++ {
		ps[i] = NewMockPlugin(fmt.Sprintf("plugin-%d", i), "gatherer", nil)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = r.Register(ps[i])
	}
}

func BenchmarkRegistry_Get(b *testing.B) {
	r := NewRegistry()

	for i := 0; i < 1000; i++ {
		p := NewMockPlugin(fmt.Sprintf("plugin-%d", i), "gatherer", nil)
		_ = r.Register(p)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = r.Get(fmt.Sprintf("plugin-%d", i%1000))
	}
}

func BenchmarkRegistry_GetLoadOrder(b *testing.B) {
	r := NewRegistry()

	for i := 0; i < 100; i++ {
		deps := make([]string, 0, 1)
		if i > 0 {
			deps = append(deps, fmt.Sprintf("plugin-%d", i-1))
		}
		p := NewMockPlugin(fmt.Sprintf("plugin-%d", i), "gatherer", deps)
		_ = r.Register(p)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = r.GetLoadOrder()
	}
}

func BenchmarkManager_Initialize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		r := NewRegistry()
		p := NewMockPlugin("test-plugin", "gatherer", nil)
		_ = r.Register(p)

		config := ManagerConfig{
			InitTimeout:  5 * time.Second,
			StartTimeout: 5 * time.Second,
			StopTimeout:  5 * time.Second,
		}

		manager := NewManager(config, r)
		b.StartTimer()

		_ = manager.Initialize(make(map[string]map[string]interface{}))
	}
}
