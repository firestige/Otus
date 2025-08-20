package api

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/plugin"
)

type Config struct {
	*config.CommonFields

	ReporterConfig   []plugin.Config `mapstructure:"reporters"`
	FallbackerConfig plugin.Config   `mapstructure:"fallbacker"`
	ClientName       string          `mapstructure:"client_name"`

	MaxQueueSize     int `mapstructure:"max_queue_size"`
	MinFlushInterval int `mapstructure:"min_flush_interval"`
	FlushInterval    int `mapstructure:"flush_interval"`
}
