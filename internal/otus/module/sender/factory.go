package sender

import (
	"sync"

	sender "firestige.xyz/otus/internal/otus/module/sender/api"
	fallbacker "firestige.xyz/otus/plugins/fallbacker/api"
	reporter "firestige.xyz/otus/plugins/reporter/api"
)

func NewSender(cfg *sender.Config) sender.Sender {
	s := &Sender{
		config:       cfg,
		reporters:    make([]reporter.Reporter, 0),
		shutdownOnce: &sync.Once{},
	}
	for _, r := range s.config.ReporterConfig {
		s.reporters = append(s.reporters, reporter.GetReporter(r))
	}
	s.fallbacker = fallbacker.GetFallbacker(cfg.FallbackerConfig)
	return s
}
