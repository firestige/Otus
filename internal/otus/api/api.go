package api

import (
	"net"

	"github.com/google/gopacket/layers"
)

type Exchange struct {
	packet  *NetPacket
	context map[string]interface{}
}

type NetPacket struct {
	FiveTuple *FiveTuple
	Timestamp int64  //accurate to nanosecond
	Protocol  string // application protocol (e.g. SIP, RTP)
	Payload   []byte // content
}

type FiveTuple struct {
	SrcIP    net.IP
	SrcPort  uint16
	DstIP    net.IP
	DstPort  uint16
	Protocol layers.IPProtocol // IP protocol (e.g., TCP, UDP)
}
