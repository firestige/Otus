// Package decoder implements L2-L4 protocol stack decoding.
package decoder

import "firestige.xyz/otus/internal/core"

// Decoder decodes raw packets into structured format.
type Decoder interface {
Decode(raw core.RawPacket) (core.DecodedPacket, error)
}
