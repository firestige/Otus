package codec

import (
	"context"
	"fmt"
	"time"

	"firestige.xyz/otus/internal/otus/api"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type Decoder struct {
	ipv4Reassembler *IPv4Reassembler
	handler         TransportHandler
	parser          *gopacket.DecodingLayerParser
	ipv4            layers.IPv4
	tcp             layers.TCP
	udp             layers.UDP

	output chan *api.NetPacket
}

func NewDecoder(output chan *api.NetPacket, opts *Options, ctx context.Context) *Decoder {
	tcphandler := &tcpHandler{}
	udpHandler := &udpHandler{}
	d := &Decoder{
		output:  output,
		handler: NewTransportHandlerComposite(tcphandler, udpHandler),
	}
	dlp := gopacket.NewDecodingLayerParser(
		layers.LayerTypeEthernet,
		&d.ipv4,
		&d.tcp,
		&d.udp)
	d.parser = dlp
	d.ipv4Reassembler = NewIPv4Reassembler(ReassemblerOptions{
		MaxAge:       30 * time.Second,
		MaxFragments: 100,
		MaxIPSize:    65535,
	})
	return d
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
	decodedLayers := make([]gopacket.LayerType, 0, 10)
	d.parser.DecodeLayers(data, &decodedLayers)
	for _, layer := range decodedLayers {
		if layer == layers.LayerTypeIPv4 {
			packet, err := d.ipv4Reassembler.ProcessIPv4Packet(&d.ipv4, ci)
			if err != nil {
				return nil, err
			}
			msg, err := d.parseTransportLayer(packet, ci)
			if err != nil {
				return nil, err
			}
			return msg, nil
		}
	}
}

func (d *Decoder) parseTransportLayer(packet *IPv4Packet, ci *gopacket.CaptureInfo) error {
	switch packet.Protocol {
	case layers.IPProtocolTCP:
		// 转入tcpassenbler处理TCP分段,分段会在Assemble中通过ReassembledChan被处理并发送至pktqueue
		d.tcpReassembler.AssembleWithTimestamp(flow, &d.tcp, ci.Timestamp)
		return nil
	case layers.IPProtocolUDP:
		// 转入udp协议处理器，udp消息被识别后直接转换成对应的应用层协议并发送至pktqueue
		d.udp.ProcessUDPPacket(packet, ci)
	default:
		return fmt.Errorf("unsupported transport protocol: %v", packet.Protocol)
	}
}
