package capture

import (
	"context"
	"encoding/json"

	"firestige.xyz/otus/internal/log"
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

func NewCapture(ctx context.Context, cfg *capture.Config) capture.Capture {
	partitionCount := cfg.Partition
	if partitionCount < 1 {
		partitionCount = 1
	}

	// 使用 json.MarshalIndent 递归打印（注意：仅导出字段会被序列化）
	if b, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		log.GetLogger().Infof("capture config: %s", string(b))
	} else {
		log.GetLogger().WithField("err", err).Infof("capture config marshal error")
	}

	ctx, cancel := context.WithCancel(ctx)
	capture := &Capture{
		config:         cfg,
		partitionCount: partitionCount,
		partitions:     make([]*Partition, partitionCount),
		outputChannels: make([]chan<- *otus.OutputPacketContext, partitionCount),
		ctx:            ctx,
		cancel:         cancel,
	}

	for i := 0; i < partitionCount; i++ {
		capture.partitions[i] = &Partition{
			id:            i,
			fanoutGroupID: uint16(cfg.HandleConfig.FanoutId),
			// 推迟partition初始化到Capture.PostConstruct中完成，那时再进行handle、decoder和共享channel的绑定
		}
	}
	return capture
}

// func buildPartition(id int, cfg *capture.Config) *Partition {
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
// 	handle, _ := handle.HandleFactory().CreateHandle(cfg.HandleConfig)
// 	return &Partition{
// 		id:            id,
// 		fanoutGroupID: uint16(cfg.HandleConfig.FanoutId),
// 		handle:        handle,
// 		decoder:       decoder,
// 	}
// }
