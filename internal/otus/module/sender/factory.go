package sender

import (
	"firestige.xyz/otus/internal/otus/module/sender/api"
	reporter "firestige.xyz/otus/plugins/reporter/api"
)

func NewSender(cfg *api.Config) api.Sender {
	s := &Sender{}
	for _, r := range s.config.ReporterConfig {
		s.reporters = append(s.reporters, reporter.GetReporter(r))
	}
	return s
}
