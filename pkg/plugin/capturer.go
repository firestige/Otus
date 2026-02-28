// Package plugin defines plugin interfaces.
package plugin

import (
	"context"

	"icc.tech/capture-agent/internal/core"
)

// Capturer captures raw packets from network interface.
type Capturer interface {
	Plugin
	Capture(ctx context.Context, output chan<- core.RawPacket) error
	Stats() CaptureStats
}

// CaptureStats represents capture statistics.
type CaptureStats struct {
	PacketsReceived  uint64
	PacketsDropped   uint64
	PacketsIfDropped uint64
}
