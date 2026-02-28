// Package task implements task management.
package task

import (
	"sync"
	"sync/atomic"

	"icc.tech/capture-agent/pkg/plugin"
)

// FlowRegistry provides per-Task flow state storage.
// It is shared across all pipelines within a task and is thread-safe.
// Typical use case: SIP parser tracking INVITE → 200 OK → ACK dialog state.
type FlowRegistry struct {
	data  sync.Map // map[plugin.FlowKey]any - stores arbitrary flow state
	count atomic.Int64
}

// NewFlowRegistry creates a new flow registry.
func NewFlowRegistry() *FlowRegistry {
	return &FlowRegistry{}
}

// Get retrieves flow state for the given key.
// Returns (value, true) if found, (nil, false) otherwise.
func (r *FlowRegistry) Get(key plugin.FlowKey) (any, bool) {
	return r.data.Load(key)
}

// Set stores flow state for the given key.
// Overwrites existing value if present.
func (r *FlowRegistry) Set(key plugin.FlowKey, value any) {
	_, loaded := r.data.Swap(key, value)
	if !loaded {
		r.count.Add(1)
	}
}

// Delete removes flow state for the given key.
func (r *FlowRegistry) Delete(key plugin.FlowKey) {
	_, loaded := r.data.LoadAndDelete(key)
	if loaded {
		r.count.Add(-1)
	}
}

// Range iterates over all flows in the registry.
// f should return true to continue iteration or false to stop.
func (r *FlowRegistry) Range(f func(key plugin.FlowKey, value any) bool) {
	r.data.Range(func(k, v any) bool {
		flowKey, ok := k.(plugin.FlowKey)
		if !ok {
			return true // Skip invalid keys
		}
		return f(flowKey, v)
	})
}

// Count returns the number of flows in the registry.
// O(1) via atomic counter maintained by Set/Delete/Clear.
func (r *FlowRegistry) Count() int {
	return int(r.count.Load())
}

// Clear removes all flows from the registry.
func (r *FlowRegistry) Clear() {
	r.data.Range(func(key, _ any) bool {
		r.data.Delete(key)
		r.count.Add(-1)
		return true
	})
}
