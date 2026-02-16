// Package plugin defines plugin interfaces.
package plugin

import (
	"context"

	"firestige.xyz/otus/internal/core"
)

// Reporter sends output packets to external systems.
type Reporter interface {
	Plugin
	Report(ctx context.Context, pkt *core.OutputPacket) error
	Flush(ctx context.Context) error
}
