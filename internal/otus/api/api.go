package api

import "net"

type NetPacket struct {
	Protocol  byte  // IP protocol (e.g., TCP, UDP, STCP)
	direction uint8 // 0: inbound, 1: outbound
	SrcIP     net.IP
	DstIP     net.IP
	SrcPort   uint16
	DstPort   uint16
	Timestamp uint64 //accurate to nanosecond
	ProtoType byte   // application protocol (e.g. SIP, RTP)
	Payload   []byte // content
}
