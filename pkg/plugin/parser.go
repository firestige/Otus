// Package plugin defines plugin interfaces.
package plugin

import (
	"net/netip"
	"time"

	"icc.tech/capture-agent/internal/core"
)

// Parser parses application-layer protocols.
type Parser interface {
	Plugin
	CanHandle(pkt *core.DecodedPacket) bool
	Handle(pkt *core.DecodedPacket) (payload any, labels core.Labels, err error)
}

// FlowRegistry is the interface for per-Task flow state storage.
// Parsers can use this to track state across packets in a flow.
type FlowRegistry interface {
	Get(key FlowKey) (any, bool)
	Set(key FlowKey, value any)
	Delete(key FlowKey)
	Range(f func(key FlowKey, value any) bool)
	Count() int
	Clear()
}

// FlowKey uniquely identifies a network flow using 5-tuple.
// Used as a map key — must remain comparable (no slices/maps/time.Time).
type FlowKey struct {
	SrcIP   netip.Addr
	DstIP   netip.Addr
	SrcPort uint16
	DstPort uint16
	Proto   uint8
}

// ReverseKey returns the flow key with src and dst swapped.
// Used for bidirectional registry lookups so that a packet arriving in the
// opposite direction of the registered key still resolves to the same session.
func (k FlowKey) ReverseKey() FlowKey {
	return FlowKey{
		SrcIP:   k.DstIP,
		DstIP:   k.SrcIP,
		SrcPort: k.DstPort,
		DstPort: k.SrcPort,
		Proto:   k.Proto,
	}
}

// FlowEntry is the value stored in the FlowRegistry for each media flow.
// It carries session correlation data and a creation timestamp so that
// stale entries from a previous call on the same ports can be rejected.
type FlowEntry struct {
	// CallID is the SIP Call-ID header value from the INVITE/200 OK exchange.
	CallID string
	// Codec is the codec name from SDP (e.g. "PCMU/8000"), may be empty.
	Codec string
	// RegisteredAt is the wall-clock time at which the entry was stored.
	// Callers compare this against the session TTL to discard stale entries
	// that linger after BYE/CANCEL when port numbers are reused by a new call.
	RegisteredAt time.Time
}

// FlowRegistryAware is an optional interface that parsers can implement
// to receive a FlowRegistry during the Wire phase.
type FlowRegistryAware interface {
	SetFlowRegistry(registry FlowRegistry)
}
