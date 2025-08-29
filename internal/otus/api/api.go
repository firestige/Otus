package api

import (
	"fmt"
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

type BatchPacket []*NetPacket

// 消息上下文

type OutputPacketContext struct {
	Context map[string]*NetPacket
}

func (c *OutputPacketContext) Get(applicationProtocol string) (*NetPacket, error) {
	p, ok := c.Context[applicationProtocol]
	if !ok {
		err := fmt.Errorf("packet not found")
		return nil, err
	}
	return p, nil
}

func (c *OutputPacketContext) Set(packet *NetPacket) {
	applicationProtocol := string(packet.ApplicationProtoType)
	c.Context[applicationProtocol] = packet
}
