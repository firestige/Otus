package sender

import (
	"sync"

	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/module/buffer"
	sender "firestige.xyz/otus/internal/otus/module/sender/api"
	fallbacker "firestige.xyz/otus/plugins/fallbacker/api"
	reporter "firestige.xyz/otus/plugins/reporter/api"
)

func NewSender(cfg *sender.Config) sender.Sender {
	s := &Sender{
		config:       cfg,
		reporters:    make([]reporter.Reporter, 0),
		fallbacker:   fallbacker.GetFallbacker(cfg.FallbackerConfig),
		shutdownOnce: &sync.Once{},
	}
	for _, r := range s.config.ReporterConfig {
		s.reporters = append(s.reporters, reporter.GetReporter(r))
	}
	s.inputs = make([]<-chan *otus.OutputPacketContext, s.config.Partition)
	s.buffers = make([]*buffer.BatchBuffer, s.config.Partition)
	s.flushChannel = make([]chan *buffer.BatchBuffer, s.config.Partition)
	return s
}
