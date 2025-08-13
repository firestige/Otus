package codec

import "net"

type Packet struct {
	Protocol  string
	Transport string
	IpVersion string
	SrcIP     net.IP
	DstIP     net.IP
	SrcPort   uint16
	DstPort   uint16
	Tsec      uint32
	Tmsec     uint32
	Payload   []byte
}

type Decoder interface {
	Decode(data []byte) (*Packet, error)
}
