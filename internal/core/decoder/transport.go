// Package decoder implements protocol decoding.
package decoder

import (
	"encoding/binary"

	"icc.tech/capture-agent/internal/core"
)

const (
	udpHeaderLen = 8

	// Protocol numbers
	protocolUDP = 17
)

// decodeTransport decodes transport layer header (UDP only).
// Non-UDP protocols return the raw data as payload with no transport header.
func decodeTransport(data []byte, protocol uint8) (core.TransportHeader, []byte, error) {
	if protocol == protocolUDP {
		return decodeUDP(data)
	}
	// Unsupported transport protocol — pass data through.
	return core.TransportHeader{Protocol: protocol}, data, nil
}

// decodeUDP decodes UDP header.
func decodeUDP(data []byte) (core.TransportHeader, []byte, error) {
	if len(data) < udpHeaderLen {
		return core.TransportHeader{}, nil, core.ErrPacketTooShort
	}

	transport := core.TransportHeader{
		Protocol: protocolUDP,
	}

	// Source Port (2 bytes at offset 0)
	transport.SrcPort = binary.BigEndian.Uint16(data[0:2])

	// Destination Port (2 bytes at offset 2)
	transport.DstPort = binary.BigEndian.Uint16(data[2:4])

	// Length (2 bytes at offset 4) - includes header and data
	// Checksum (2 bytes at offset 6) - not needed for decoding

	// Payload starts after UDP header
	payload := data[udpHeaderLen:]
	return transport, payload, nil
}
