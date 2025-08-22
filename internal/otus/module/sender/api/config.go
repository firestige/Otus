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

	MaxBufferSize  int `mapstructure:"max_buffer_size"`
	MinFlushEvents int `mapstructure:"min_flush_events"`
	FlushInterval  int `mapstructure:"flush_interval"`
}
