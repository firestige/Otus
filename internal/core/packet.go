// Package core defines core data structures with zero external dependencies.
package core

import (
	"net/netip"
	"time"
)

// RawPacket is captured from the network interface, zero-copy reference to ring buffer.
type RawPacket struct {
	Data           []byte    // Raw frame data, zero-copy slice
	Timestamp      time.Time // Capture timestamp (kernel timestamp preferred)
	CaptureLen     uint32    // Actual captured length
	OrigLen        uint32    // Original frame length
	InterfaceIndex int       // Network interface index
}

// DecodedPacket is the result of L2-L4 protocol stack decoding.
type DecodedPacket struct {
	Timestamp   time.Time
	Ethernet    EthernetHeader
	IP          IPHeader
	Transport   TransportHeader
	Payload     []byte // Application layer payload, zero-copy slice
	CaptureLen  uint32
	OrigLen     uint32
	Reassembled bool // Whether packet went through IP fragment reassembly
}

// OutputPacket is the final output sent to reporters.
type OutputPacket struct {
	// Envelope
	TaskID     string
	AgentID    string
	PipelineID int
	Timestamp  time.Time

	// Network context
	SrcIP    netip.Addr
	DstIP    netip.Addr
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8

	// Labels — Parser / Processor annotations
	Labels Labels

	// Typed Payload — Parser parsing result
	PayloadType string // e.g. "sip", "rtp", "raw"
	Payload     any    // Concrete type determined by PayloadType, Reporter does type assertion
	RawPayload  []byte // Raw payload (optional preservation)
}
