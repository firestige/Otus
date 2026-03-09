// Package filter implements a pass-through (no-op) processor.
// Every packet is kept unconditionally.  Future versions will support
// rule-based filtering via the configuration map.
package filter

import (
	"context"

	"icc.tech/capture-agent/internal/core"
	"icc.tech/capture-agent/pkg/plugin"
)

// FilterProcessor is a passthrough processor that keeps every packet.
type FilterProcessor struct{}

// NewFilterProcessor creates a new FilterProcessor instance.
func NewFilterProcessor() plugin.Processor {
	return &FilterProcessor{}
}

// Name returns the plugin identifier.
func (f *FilterProcessor) Name() string { return "filter" }

// Init accepts (and ignores) the configuration map.
func (f *FilterProcessor) Init(_ map[string]any) error { return nil }

// Start is a no-op.
func (f *FilterProcessor) Start(_ context.Context) error { return nil }

// Stop is a no-op.
func (f *FilterProcessor) Stop(_ context.Context) error { return nil }

// Process keeps every packet unconditionally.
func (f *FilterProcessor) Process(_ *core.OutputPacket) bool { return true }
