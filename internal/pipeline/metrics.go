// Package pipeline implements pipeline metrics.
package pipeline

import (
	"sync/atomic"
)

// Metrics contains per-pipeline metrics counters.
type Metrics struct {
	TaskID     string
	PipelineID int
	
	// Packet counters (using atomic for thread-safety)
	Received     atomic.Uint64
	Decoded      atomic.Uint64
	DecodeErrors atomic.Uint64
	Parsed       atomic.Uint64
	ParseErrors  atomic.Uint64
	Processed    atomic.Uint64
	Dropped      atomic.Uint64
	Reported     atomic.Uint64
	ReportErrors atomic.Uint64
}

// NewMetrics creates a new metrics instance.
func NewMetrics(taskID string, pipelineID int) *Metrics {
	return &Metrics{
		TaskID:     taskID,
		PipelineID: pipelineID,
	}
}

// Reset resets all counters to zero.
func (m *Metrics) Reset() {
	m.Received.Store(0)
	m.Decoded.Store(0)
	m.DecodeErrors.Store(0)
	m.Parsed.Store(0)
	m.ParseErrors.Store(0)
	m.Processed.Store(0)
	m.Dropped.Store(0)
	m.Reported.Store(0)
	m.ReportErrors.Store(0)
}
