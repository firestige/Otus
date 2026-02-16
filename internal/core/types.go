// Package core defines core types with zero external dependencies.
package core

import "net/netip"

// EthernetHeader represents L2 Ethernet frame header.
type EthernetHeader struct {
	SrcMAC    [6]byte
	DstMAC    [6]byte
	EtherType uint16   // 0x0800=IPv4, 0x86DD=IPv6, 0x8100=VLAN
	VLANs     []uint16 // 0~2 VLAN IDs (QinQ scenarios have 2)
}

// IPHeader represents L3 IP header (IPv4/IPv6).
type IPHeader struct {
	Version  uint8
	SrcIP    netip.Addr // Go stdlib value type, zero allocation
	DstIP    netip.Addr
	Protocol uint8  // TCP=6, UDP=17, SCTP=132
	TTL      uint8
	TotalLen uint16
	// Inner IP addresses after tunnel decapsulation (zero value if not tunneled)
	InnerSrcIP netip.Addr
	InnerDstIP netip.Addr
}

// TransportHeader represents L4 transport layer header (TCP/UDP).
type TransportHeader struct {
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8 // Redundant storage for convenience
	// TCP-specific fields (only populated for TCP)
	TCPFlags uint8
	SeqNum   uint32
	AckNum   uint32
}
