package codec

import (
	"net"
	"time"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/api"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// IPv4Packet 重组后的IPv4包
type IPv4Packet struct {
	SrcIP     net.IP
	DstIP     net.IP
	Protocol  layers.IPProtocol
	ID        uint16
	Flags     layers.IPv4Flag
	TTL       uint8
	Length    uint16
	Payload   []byte
	Timestamp time.Time
	Flow      gopacket.Flow
}

type Decoder struct {
	ipv4Reassembler *IPv4Reassembler
	handler         TransportHandler
	parser          *gopacket.DecodingLayerParser
	ipv4            layers.IPv4
	tcp             layers.TCP
	udp             layers.UDP
}

func (d *Decoder) Decode(data []byte, ci *gopacket.CaptureInfo) error {
	// 我们假设网卡帮我们解决了重传问题，不存在重复包
	// 我们假设网络请求不包含GRE、vlan等特殊报文
	// 我们假设只有IPv4流量
	// 此时我们在decode函数中只需要处理IP分片和传输层协议
	// 1. 调用gopacket对每一层预解码
	// 2. 处理IP分片
	// 3. 处理传输层协议

	// 解码流程
	//   DecodeLayers 从底层协议（如 Ethernet）开始，依次调用每一层的 DecodeFromBytes 方法。
	//   每解码出一层协议，就将该层的 LayerType 添加到 decoded 切片。
	//   解码器根据上一层的内容决定下一层的类型（如 Ethernet 的类型字段决定是 IPv4 还是 IPv6）。
	//   如果遇到分片、嵌套或未知协议，解码流程会相应处理或终止。(重要)
	log.GetLogger().Infof("IPv4 packet: %+v", ci)
	decodedLayers := make([]gopacket.LayerType, 0, 10)
	d.parser.DecodeLayers(data, &decodedLayers)
	log.GetLogger().Infof("Decoded layers: %+v", decodedLayers)
	for _, layer := range decodedLayers {
		log.GetLogger().Infof("Decoded layer: %s", layer)
		if layer == layers.LayerTypeIPv4 {
			packet, err := d.ipv4Reassembler.ProcessIPv4Packet(&d.ipv4, ci)
			if err != nil {
				return err
			}
			log.GetLogger().Infof("IPv4 packet reassembled: %+v", packet)
			d.handler.handle(packet)
		}
	}
	return nil
}

func extractFiveTuple(packet *IPv4Packet) api.FiveTuple {
	var srcPort, dstPort uint16

	payload := packet.Payload

	switch packet.Protocol {
	case layers.IPProtocolTCP:
		// TCP头至少需要20字节
		if len(payload) >= 20 {
			srcPort = uint16(payload[0])<<8 | uint16(payload[1])
			dstPort = uint16(payload[2])<<8 | uint16(payload[3])
		}
	case layers.IPProtocolUDP:
		// UDP头至少需要8字节
		if len(payload) >= 8 {
			srcPort = uint16(payload[0])<<8 | uint16(payload[1])
			dstPort = uint16(payload[2])<<8 | uint16(payload[3])
		}
	}

	return api.FiveTuple{
		SrcIP:    packet.SrcIP,
		DstIP:    packet.DstIP,
		SrcPort:  srcPort,
		DstPort:  dstPort,
		Protocol: packet.Protocol,
	}
}
