// Package models re-exports core types for external use.
package models

import "firestige.xyz/otus/internal/core"

// Re-export core packet types for plugins
type (
RawPacket     = core.RawPacket
DecodedPacket = core.DecodedPacket
OutputPacket  = core.OutputPacket
Labels        = core.Labels
)
