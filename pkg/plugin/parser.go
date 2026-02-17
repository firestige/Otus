// Package plugin defines plugin interfaces.
package plugin

import (
	"net/netip"

	"firestige.xyz/otus/internal/core"
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
type FlowKey struct {
	SrcIP   netip.Addr
	DstIP   netip.Addr
	SrcPort uint16
	DstPort uint16
	Proto   uint8
}

// FlowRegistryAware is an optional interface that parsers can implement
// to receive a FlowRegistry during the Wire phase.
type FlowRegistryAware interface {
	SetFlowRegistry(registry FlowRegistry)
}
