package codec

import (
	"context"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func NewDecoder(ctx context.Context, opts *Options) *Decoder {
	d := &Decoder{}
	dlp := gopacket.NewDecodingLayerParser(
		layers.LayerTypeEthernet,
		&layers.Ethernet{},
		&d.ipv4,
		&d.tcp,
		&d.udp)
	d.parser = dlp
	d.ipv4Reassembler = NewIPv4Reassembler(ctx, ReassemblerOptions{
		MaxAge:       time.Duration(opts.MaxAge) * time.Second,
		MaxFragments: opts.MaxFragmentsNum,
		MaxIPSize:    opts.MaxIPSize,
	})
	return d
}

func (d *Decoder) SetTransportHandler(handler TransportHandler) {
	d.handler = handler
}

func (d *Decoder) PostConstruct() error {
	return nil
}
