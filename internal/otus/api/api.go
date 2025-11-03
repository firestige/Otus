package api

import (
	"net"

	"github.com/google/gopacket/layers"
)

type ComponentType string

const (
	ComponentTypeSource    ComponentType = "source"
	ComponentTypeDecoder   ComponentType = "decoder"
	ComponentTypeProcessor ComponentType = "processor"
	ComponentTypeSink      ComponentType = "sink"
)

type Exchange struct {
	Packet  *NetPacket
	Context map[string]interface{}
}

type NetPacket struct {
	FiveTuple *FiveTuple
	Timestamp int64  //accurate to nanosecond
	Protocol  string // application protocol (e.g. SIP, RTP)
	Raw       []byte // raw packet bytes including link layer header
}

type FiveTuple struct {
	SrcIP    net.IP
	SrcPort  uint16
	DstIP    net.IP
	DstPort  uint16
	Protocol layers.IPProtocol // IP protocol (e.g., TCP, UDP)
}
