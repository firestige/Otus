package capture

import (
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/module/capture/api"
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/otus/module/capture/sniffer"
)

func NewCapture(cfg *api.Config) api.Capture {
	return Capture{
		SnifferOpt:  &sniffer.Options{},
		CodecOpt:    &codec.Options{},
		packetQueue: make(chan *otus.NetPacket, 100),
	}
}
