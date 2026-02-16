// Package decoder implements protocol decoding.
package decoder

import (
	"encoding/binary"
	"net/netip"

	"firestige.xyz/otus/internal/core"
)

const (
	ipv4HeaderMinLen = 20
	ipv6HeaderLen    = 40
)

// decodeIP decodes IP header (IPv4 or IPv6).
// Returns IPHeader and remaining payload.
func decodeIP(data []byte) (core.IPHeader, []byte, error) {
	if len(data) < 1 {
		return core.IPHeader{}, nil, core.ErrPacketTooShort
	}

	// Check IP version (first 4 bits)
	version := data[0] >> 4

	switch version {
	case 4:
		return decodeIPv4(data)
	case 6:
		return decodeIPv6(data)
	default:
		return core.IPHeader{}, nil, core.ErrUnsupportedProto
	}
}

// decodeIPv4 decodes IPv4 header.
func decodeIPv4(data []byte) (core.IPHeader, []byte, error) {
	if len(data) < ipv4HeaderMinLen {
		return core.IPHeader{}, nil, core.ErrPacketTooShort
	}

	// IHL (Internet Header Length) - lower 4 bits of first byte
	ihl := uint8(data[0] & 0x0F)
	headerLen := int(ihl * 4) // IHL is in 32-bit words

	if headerLen < ipv4HeaderMinLen || len(data) < headerLen {
		return core.IPHeader{}, nil, core.ErrPacketTooShort
	}

	ip := core.IPHeader{
		Version: 4,
	}

	// Total Length (2 bytes at offset 2)
	ip.TotalLen = binary.BigEndian.Uint16(data[2:4])

	// TTL (1 byte at offset 8)
	ip.TTL = data[8]

	// Protocol (1 byte at offset 9)
	ip.Protocol = data[9]

	// Source IP (4 bytes at offset 12)
	srcIPBytes := data[12:16]
	addr, ok := netip.AddrFromSlice(srcIPBytes)
	if !ok {
		return ip, nil, core.ErrPacketTooShort
	}
	ip.SrcIP = addr

	// Destination IP (4 bytes at offset 16)
	dstIPBytes := data[16:20]
	addr, ok = netip.AddrFromSlice(dstIPBytes)
	if !ok {
		return ip, nil, core.ErrPacketTooShort
	}
	ip.DstIP = addr

	// Payload starts after IP header
	payload := data[headerLen:]
	return ip, payload, nil
}

// decodeIPv6 decodes IPv6 header.
func decodeIPv6(data []byte) (core.IPHeader, []byte, error) {
	if len(data) < ipv6HeaderLen {
		return core.IPHeader{}, nil, core.ErrPacketTooShort
	}

	ip := core.IPHeader{
		Version: 6,
	}

	// Payload Length (2 bytes at offset 4)
	payloadLen := binary.BigEndian.Uint16(data[4:6])
	ip.TotalLen = uint16(ipv6HeaderLen) + payloadLen

	// Next Header (1 byte at offset 6) - equivalent to Protocol in IPv4
	ip.Protocol = data[6]

	// Hop Limit (1 byte at offset 7) - equivalent to TTL in IPv4
	ip.TTL = data[7]

	// Source IP (16 bytes at offset 8)
	srcIPBytes := data[8:24]
	addr, ok := netip.AddrFromSlice(srcIPBytes)
	if !ok {
		return ip, nil, core.ErrPacketTooShort
	}
	ip.SrcIP = addr

	// Destination IP (16 bytes at offset 24)
	dstIPBytes := data[24:40]
	addr, ok = netip.AddrFromSlice(dstIPBytes)
	if !ok {
		return ip, nil, core.ErrPacketTooShort
	}
	ip.DstIP = addr

	// TODO: Handle IPv6 extension headers if needed
	// For now, assume no extension headers and payload starts at offset 40

	payload := data[ipv6HeaderLen:]
	return ip, payload, nil
}

// isIPFragment checks if an IP packet is a fragment.
func isIPFragment(ipData []byte, version uint8) bool {
	if version == 4 {
		if len(ipData) < ipv4HeaderMinLen {
			return false
		}
		// Flags and Fragment Offset (2 bytes at offset 6)
		flagsOffset := binary.BigEndian.Uint16(ipData[6:8])
		moreFragments := (flagsOffset & 0x2000) != 0 // MF flag
		fragmentOffset := flagsOffset & 0x1FFF       // Fragment offset
		return moreFragments || fragmentOffset != 0
	}
	// IPv6 fragmentation is handled via extension headers
	// Simplified: assume no fragmentation for now
	return false
}
