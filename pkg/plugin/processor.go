// Package plugin defines plugin interfaces.
package plugin

import "firestige.xyz/otus/internal/core"

// Processor processes output packets.
type Processor interface {
Plugin
Process(pkt *core.OutputPacket) (keep bool)
}
