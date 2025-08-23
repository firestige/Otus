package capture

import (
	"time"

	otus "firestige.xyz/otus/internal/otus/api"
	capture "firestige.xyz/otus/internal/otus/module/capture/api"
)

// func NewCapture(cfg *capture.Config) capture.Capture {
// 	c := &Capture{
// 		SnifferOpt:  &sniffer.Options{},
// 		CodecOpt:    &codec.Options{},
// 	}

// 	parsers := make([]codec.Parser, 0)
// 	for _, c := range cfg.ParserConfig {
// 		// TODO satellite 如何实现只需要name 就可以加载的？
// 		parsers = append(parsers, parser.GetParser(c))
// 	}
// 	parser := codec.NewParserComposite(parsers...)
// 	tcphandler := codec.NewTCPHandler(c.packetQueue, parser)
// 	udphandler := codec.NewUDPHandler(c.packetQueue, parser)
// 	handler := codec.NewTransportHandlerComposite(tcphandler, udphandler)
// 	decoder := codec.NewDecoder(cfg.CodecConfig)
// 	decoder.SetTransportHandler(handler)
// 	sniffer := sniffer.NewSniffer(cfg.SnifferConfig)
// 	sniffer.SetDecoder(decoder)
// 	c.sniffer = sniffer
// 	return c
// }

func NewCapture(cfg *capture.Config) capture.Capture {
	partitionCount := cfg.Partitions
	if partitionCount < 1 {
		partitionCount = 1
	}

	capture := &Capture{
		config:         cfg,
		partitionCount: partitionCount,
		partitions:     make([]*Partition, partitionCount),
		outputChannels: make([]chan *otus.BatchePacket, partitionCount),
	}

	for i := 0; i < partitionCount; i++ {
		capture.partitions[i] = &Partition{
			id:            i,
			fanoutGroupID: uint16(cfg.HandleConfig.FanoutId),
			batchSize:     cfg.batchSize,
			batchTimeout:  time.Duration(cfg.HandleConfig.Timeout) * time.Millisecond,
			currentBatch:  make([]*otus.BatchePacket, 0, cfg.batchSize),
		}
		capture.outputChannels[i] = capture.partitions[i].outputCh
	}
	return capture
}
