// Package decoder implements protocol decoding.
package decoder

import (
	"encoding/binary"

	"firestige.xyz/otus/internal/core"
)

const (
	// Protocol numbers
	protocolGRE  = 47
	protocolIPIP = 4

	// Well-known UDP ports
	vxlanPort  = 4789
	genevePort = 6081

	// Header lengths
	vxlanHeaderLen  = 8
	geneveHeaderLen = 8
	greHeaderMinLen = 4
)

// decodeTunnel attempts to decapsulate tunnel protocols.
// Returns inner IP header and payload, or zero-value if not a tunnel.
func decodeTunnel(data []byte, protocol uint8) (core.IPHeader, []byte, error) {
	switch protocol {
	case protocolGRE:
		return decodeGRE(data)
	case protocolIPIP:
		return decodeIPIP(data)
	case protocolUDP:
		// Check for VXLAN or Geneve based on port
		// Need to parse UDP header first
		if len(data) >= 8 {
			dstPort := binary.BigEndian.Uint16(data[2:4])
			udpPayload := data[8:]

			if dstPort == vxlanPort {
				return decodeVXLAN(udpPayload)
			} else if dstPort == genevePort {
				return decodeGeneve(udpPayload)
			}
		}
		return core.IPHeader{}, data, nil
	default:
		return core.IPHeader{}, data, nil
	}
}

// decodeVXLAN decapsulates VXLAN tunnel.
func decodeVXLAN(data []byte) (core.IPHeader, []byte, error) {
	if len(data) < vxlanHeaderLen {
		return core.IPHeader{}, data, nil
	}

	// VXLAN header format:
	// 0-3: Flags (1 byte) + Reserved (3 bytes)
	// 4-7: VNI (3 bytes) + Reserved (1 byte)

	// Check if VNI flag is set (bit 3 of first byte)
	flags := data[0]
	if (flags & 0x08) == 0 {
		// Invalid VXLAN packet
		return core.IPHeader{}, data, nil
	}

	// Skip VXLAN header (8 bytes)
	// Inner Ethernet frame starts after VXLAN header
	innerFrame := data[vxlanHeaderLen:]

	// Skip inner Ethernet header (assume 14 bytes, no VLAN)
	if len(innerFrame) < ethernetHeaderLen {
		return core.IPHeader{}, data, nil
	}

	// Get inner EtherType
	etherType := binary.BigEndian.Uint16(innerFrame[12:14])
	if etherType != etherTypeIPv4 && etherType != etherTypeIPv6 {
		return core.IPHeader{}, data, nil
	}

	// Decode inner IP packet
	innerIP, payload, err := decodeIP(innerFrame[ethernetHeaderLen:])
	if err != nil {
		return core.IPHeader{}, data, nil
	}

	return innerIP, payload, nil
}

// decodeGeneve decapsulates Geneve tunnel.
func decodeGeneve(data []byte) (core.IPHeader, []byte, error) {
	if len(data) < geneveHeaderLen {
		return core.IPHeader{}, data, nil
	}

	// Geneve header format (simplified):
	// 0: Version (2 bits) + Opt Len (6 bits)
	// 1: Flags
	// 2-3: Protocol Type
	// 4-6: VNI
	// 7: Reserved

	version := data[0] >> 6
	if version != 0 {
		// Unsupported version
		return core.IPHeader{}, data, nil
	}

	optLen := data[0] & 0x3F
	headerLen := geneveHeaderLen + int(optLen)*4

	if len(data) < headerLen {
		return core.IPHeader{}, data, nil
	}

	// Skip Geneve header + options
	innerFrame := data[headerLen:]

	// Skip inner Ethernet header
	if len(innerFrame) < ethernetHeaderLen {
		return core.IPHeader{}, data, nil
	}

	// Get inner EtherType
	etherType := binary.BigEndian.Uint16(innerFrame[12:14])
	if etherType != etherTypeIPv4 && etherType != etherTypeIPv6 {
		return core.IPHeader{}, data, nil
	}

	// Decode inner IP packet
	innerIP, payload, err := decodeIP(innerFrame[ethernetHeaderLen:])
	if err != nil {
		return core.IPHeader{}, data, nil
	}

	return innerIP, payload, nil
}

// decodeGRE decapsulates GRE tunnel.
func decodeGRE(data []byte) (core.IPHeader, []byte, error) {
	if len(data) < greHeaderMinLen {
		return core.IPHeader{}, data, nil
	}

	// GRE header format:
	// 0-1: Flags and Version
	// 2-3: Protocol Type

	flags := binary.BigEndian.Uint16(data[0:2])
	protocolType := binary.BigEndian.Uint16(data[2:4])

	// Calculate GRE header length based on flags
	headerLen := greHeaderMinLen

	// Checksum present (bit 15)
	if (flags & 0x8000) != 0 {
		headerLen += 4
	}
	// Key present (bit 13)
	if (flags & 0x2000) != 0 {
		headerLen += 4
	}
	// Sequence present (bit 12)
	if (flags & 0x1000) != 0 {
		headerLen += 4
	}

	if len(data) < headerLen {
		return core.IPHeader{}, data, nil
	}

	// Check if GRE payload is IP
	if protocolType != 0x0800 && protocolType != 0x86DD {
		// Not IP, return as-is
		return core.IPHeader{}, data, nil
	}

	// Decode inner IP packet
	innerIP, payload, err := decodeIP(data[headerLen:])
	if err != nil {
		return core.IPHeader{}, data, nil
	}

	return innerIP, payload, nil
}

// decodeIPIP decapsulates IPIP tunnel.
func decodeIPIP(data []byte) (core.IPHeader, []byte, error) {
	// IPIP is IP-in-IP encapsulation
	// Outer IP payload is directly the inner IP packet
	return decodeIP(data)
}
