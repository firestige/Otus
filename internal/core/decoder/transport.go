// Package decoder implements protocol decoding.
package decoder

import (
	"encoding/binary"

	"firestige.xyz/otus/internal/core"
)

const (
	udpHeaderLen    = 8
	tcpHeaderMinLen = 20

	// Protocol numbers
	protocolTCP = 6
	protocolUDP = 17
)

// decodeTransport decodes transport layer header (TCP/UDP).
// Returns TransportHeader and remaining payload.
func decodeTransport(data []byte, protocol uint8) (core.TransportHeader, []byte, error) {
	switch protocol {
	case protocolTCP:
		return decodeTCP(data)
	case protocolUDP:
		return decodeUDP(data)
	default:
		// Unsupported transport protocol (e.g., SCTP, ICMP)
		return core.TransportHeader{Protocol: protocol}, data, nil
	}
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

// decodeTCP decodes TCP header.
func decodeTCP(data []byte) (core.TransportHeader, []byte, error) {
	if len(data) < tcpHeaderMinLen {
		return core.TransportHeader{}, nil, core.ErrPacketTooShort
	}

	transport := core.TransportHeader{
		Protocol: protocolTCP,
	}

	// Source Port (2 bytes at offset 0)
	transport.SrcPort = binary.BigEndian.Uint16(data[0:2])

	// Destination Port (2 bytes at offset 2)
	transport.DstPort = binary.BigEndian.Uint16(data[2:4])

	// Sequence Number (4 bytes at offset 4)
	transport.SeqNum = binary.BigEndian.Uint32(data[4:8])

	// Acknowledgment Number (4 bytes at offset 8)
	transport.AckNum = binary.BigEndian.Uint32(data[8:12])

	// Data Offset (4 bits at offset 12, upper 4 bits)
	dataOffset := uint8(data[12] >> 4)
	headerLen := int(dataOffset * 4) // Data offset is in 32-bit words

	if headerLen < tcpHeaderMinLen || len(data) < headerLen {
		return transport, nil, core.ErrPacketTooShort
	}

	// TCP Flags (lower 6 bits of byte 13)
	// Byte 13: | reserved (2 bits) | flags (6 bits) |
	// Flags: URG, ACK, PSH, RST, SYN, FIN
	transport.TCPFlags = data[13] & 0x3F

	// Payload starts after TCP header (including options)
	payload := data[headerLen:]
	return transport, payload, nil
}
