// Package plugin defines plugin interfaces.
package plugin

import "firestige.xyz/otus/internal/core"

// Parser parses application-layer protocols.
type Parser interface {
Plugin
CanHandle(pkt *core.DecodedPacket) bool
Handle(pkt *core.DecodedPacket) (payload any, labels core.Labels, err error)
}
