package capture

import (
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/module/capture/api"
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/otus/module/capture/sniffer"
)

func NewCapture(cfg *api.Config) api.Capture {
	c := &Capture{
		SnifferOpt:  &sniffer.Options{},
		CodecOpt:    &codec.Options{},
		packetQueue: make(chan *otus.NetPacket, 100),
	}
	// TODO 从 config 创建 sniff 和 codec 对象传入 capture
	return c
}
