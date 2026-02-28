// Package plugin defines plugin interfaces.
package plugin

import "icc.tech/capture-agent/internal/core"

// Processor processes output packets.
type Processor interface {
	Plugin
	Process(pkt *core.OutputPacket) (keep bool)
}
