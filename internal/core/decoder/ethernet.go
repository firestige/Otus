// Package decoder implements protocol decoding.
package decoder

import (
	"encoding/binary"

	"firestige.xyz/otus/internal/core"
)

const (
	// Ethernet constants
	ethernetHeaderLen = 14
	vlanHeaderLen     = 4

	// EtherType values
	etherTypeIPv4 = 0x0800
	etherTypeIPv6 = 0x86DD
	etherTypeVLAN = 0x8100
	etherTypeQinQ = 0x88A8
)

// decodeEthernet decodes Ethernet frame header (including VLAN tags).
// Returns EthernetHeader and remaining payload.
func decodeEthernet(data []byte) (core.EthernetHeader, []byte, error) {
	if len(data) < ethernetHeaderLen {
		return core.EthernetHeader{}, nil, core.ErrPacketTooShort
	}

	eth := core.EthernetHeader{}

	// Destination MAC (6 bytes)
	copy(eth.DstMAC[:], data[0:6])

	// Source MAC (6 bytes)
	copy(eth.SrcMAC[:], data[6:12])

	// EtherType (2 bytes)
	etherType := binary.BigEndian.Uint16(data[12:14])
	offset := ethernetHeaderLen

	// Handle VLAN tags (can be nested: QinQ)
	var vlans []uint16
	for etherType == etherTypeVLAN || etherType == etherTypeQinQ {
		if len(data) < offset+vlanHeaderLen {
			return eth, nil, core.ErrPacketTooShort
		}

		// VLAN header: 2 bytes TCI + 2 bytes EtherType
		tci := binary.BigEndian.Uint16(data[offset : offset+2])
		vlanID := tci & 0x0FFF // Lower 12 bits are VLAN ID
		vlans = append(vlans, vlanID)

		// Next EtherType
		etherType = binary.BigEndian.Uint16(data[offset+2 : offset+4])
		offset += vlanHeaderLen
	}

	eth.EtherType = etherType
	eth.VLANs = vlans

	// Validate EtherType
	if etherType != etherTypeIPv4 && etherType != etherTypeIPv6 {
		// Non-IP packet (ARP, LLDP, etc.)
		// Return successfully but with non-IP EtherType
	}

	payload := data[offset:]
	return eth, payload, nil
}
