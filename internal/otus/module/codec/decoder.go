package codec

import (
	"net"

	p "firestige.xyz/otus/pkg/pipeline"
)

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

type decoder struct {
}

func (d *decoder) decode(data p.PacketData, ci p.CaptureInfo) (*Packet, error) {
	// 这里应该实现具体的解码逻辑
	// 例如解析 IP、TCP/UDP 头部等
	// 返回一个 Packet 实例或错误
	return nil, nil // 示例返回
}
