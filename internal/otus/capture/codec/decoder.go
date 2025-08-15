package codec

import (
	"context"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// 我们首先假设不存在重复包，网络中也没有类似GRE包、VLan包、Tunnel隧道等特殊包，仅考虑IPv4环境下抓包数据
// 其次由于AF_PACKET抓包的钩子靠前，在内核处理IP包之前，所以我们很可能会看到大量的IP分片包
// 这些分片包的流量处理相对复杂，需要在解码时进行特殊处理
type Decoder struct {
	layerParser *gopacket.DecodingLayerParser
	layers      []gopacket.LayerType
}

func NewDecoder(ctx context.Context) *Decoder {
	return &Decoder{
		layerParser: gopacket.NewDecodingLayerParser(
			layers.LayerTypeEthernet,
			&layers.Ethernet{},
			&layers.IPv4{},
			&layers.UDP{},
			&layers.TCP{},
		),
		layers: make([]gopacket.LayerType, 0, 10),
	}
}

func (d *Decoder) Decode(data []byte, ci *gopacket.CaptureInfo) error {

	// 首先使用gopacket内置分层解析器解析原始报文
	d.layerParser.DecodeLayers(data, &d.layers)

	// 然后找到IP层
	var ipLayer *layers.IPv4
	for _, layer := range d.layers {
		if layer == layers.LayerTypeIPv4 {
			ipLayer = &layers.IPv4{}
			break
		}
	}

	return nil
}
