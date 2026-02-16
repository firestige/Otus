package task

import (
	"net/netip"
	"testing"

	"firestige.xyz/otus/pkg/plugin"
)

func TestFlowRegistry(t *testing.T) {
	registry := NewFlowRegistry()

	// Test Get on empty registry
	if _, ok := registry.Get(plugin.FlowKey{}); ok {
		t.Error("Expected Get to return false on empty registry")
	}

	// Test Set and Get
	key1 := plugin.FlowKey{
		SrcIP:   netip.MustParseAddr("192.168.1.1"),
		DstIP:   netip.MustParseAddr("192.168.1.2"),
		SrcPort: 5060,
		DstPort: 5060,
		Proto:   17, // UDP
	}
	value1 := "test-value-1"

	registry.Set(key1, value1)

	if v, ok := registry.Get(key1); !ok {
		t.Error("Expected Get to return true after Set")
	} else if v != value1 {
		t.Errorf("Expected value %v, got %v", value1, v)
	}

	// Test multiple keys
	key2 := plugin.FlowKey{
		SrcIP:   netip.MustParseAddr("10.0.0.1"),
		DstIP:   netip.MustParseAddr("10.0.0.2"),
		SrcPort: 80,
		DstPort: 8080,
		Proto:   6, // TCP
	}
	value2 := map[string]string{"state": "established"}

	registry.Set(key2, value2)

	if registry.Count() != 2 {
		t.Errorf("Expected count 2, got %d", registry.Count())
	}

	// Test Delete
	registry.Delete(key1)

	if _, ok := registry.Get(key1); ok {
		t.Error("Expected Get to return false after Delete")
	}

	if registry.Count() != 1 {
		t.Errorf("Expected count 1 after delete, got %d", registry.Count())
	}

	// Test Range
	count := 0
	registry.Range(func(k plugin.FlowKey, v any) bool {
		count++
		if k != key2 {
			t.Errorf("Expected key %v, got %v", key2, k)
		}
		return true
	})

	if count != 1 {
		t.Errorf("Expected Range to iterate 1 time, got %d", count)
	}

	// Test Clear
	registry.Clear()

	if registry.Count() != 0 {
		t.Errorf("Expected count 0 after Clear, got %d", registry.Count())
	}
}

func TestFlowRegistryOverwrite(t *testing.T) {
	registry := NewFlowRegistry()

	key := plugin.FlowKey{
		SrcIP:   netip.MustParseAddr("1.2.3.4"),
		DstIP:   netip.MustParseAddr("5.6.7.8"),
		SrcPort: 1234,
		DstPort: 5678,
		Proto:   17,
	}

	// Set initial value
	registry.Set(key, "value1")

	// Overwrite with new value
	registry.Set(key, "value2")

	if v, ok := registry.Get(key); !ok {
		t.Error("Expected Get to return true")
	} else if v != "value2" {
		t.Errorf("Expected overwritten value 'value2', got %v", v)
	}

	if registry.Count() != 1 {
		t.Errorf("Expected count 1 after overwrite, got %d", registry.Count())
	}
}

func TestFlowRegistryConcurrent(t *testing.T) {
	registry := NewFlowRegistry()

	key := plugin.FlowKey{
		SrcIP:   netip.MustParseAddr("1.1.1.1"),
		DstIP:   netip.MustParseAddr("2.2.2.2"),
		SrcPort: 100,
		DstPort: 200,
		Proto:   6,
	}

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			registry.Set(key, n)
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have exactly one entry (last writer wins)
	if registry.Count() != 1 {
		t.Errorf("Expected count 1, got %d", registry.Count())
	}

	// Value should be one of the written values
	if _, ok := registry.Get(key); !ok {
		t.Error("Expected Get to return true")
	}
}
