package capture

import (
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/module/capture/api"
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/otus/module/capture/sniffer"
	parser "firestige.xyz/otus/plugins/parser/api"
)

func NewCapture(cfg *api.Config) api.Capture {
	c := &Capture{
		SnifferOpt:  &sniffer.Options{},
		CodecOpt:    &codec.Options{},
		packetQueue: make(chan *otus.NetPacket, 100),
	}

	parsers := make([]codec.Parser, 0)
	for _, c := range cfg.ParserConfig {
		// TODO satellite 如何实现只需要name 就可以加载的？
		parsers = append(parsers, parser.GetParser(c))
	}
	parser := codec.NewParserComposite(parsers...)
	tcphandler := codec.NewTCPHandler(c.packetQueue, parser)
	udphandler := codec.NewUDPHandler(c.packetQueue, parser)
	handler := codec.NewTransportHandlerComposite(tcphandler, udphandler)
	decoder := codec.NewDecoder(cfg.CodecConfig)
	decoder.SetTransportHandler(handler)
	sniffer := sniffer.NewSniffer(cfg.SnifferConfig)
	sniffer.SetDecoder(decoder)
	c.sniffer = sniffer
	return c
}
