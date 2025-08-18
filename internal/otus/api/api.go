package api

import (
	"net"

	"github.com/google/gopacket/layers"
)

type NetPacket struct {
	Protocol             layers.IPProtocol // IP protocol (e.g., TCP, UDP, STCP)
	FiveTuple            *FiveTuple
	Timestamp            int64  //accurate to nanosecond
	ApplicationProtoType byte   // application protocol (e.g. SIP, RTP)
	Payload              []byte // content
}

type FiveTuple struct {
	SrcIP    net.IP
	SrcPort  uint16
	DstIP    net.IP
	DstPort  uint16
	Protocol layers.IPProtocol // IP protocol (e.g., TCP, UDP)
}
