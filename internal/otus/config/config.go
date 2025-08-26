package config

import (
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/metrics"
	capture "firestige.xyz/otus/internal/otus/module/capture/api"
	"firestige.xyz/otus/internal/otus/module/pipeline"
	"firestige.xyz/otus/internal/plugin"
)

type OtusConfig struct {
	Logger   *log.LoggerConfig  `mapstructure:"log"`
	Sharable *SharableConfig    `mapstructure:"sharable"`
	Pipes    []*pipeline.Config `mapstructure:"pipes"`
	Metrics  *metrics.Config    `mapstructure:"metrics"`
}

type SharableConfig struct {
	Capture *capture.Config `mapstructure:"capture"`
	Clients []plugin.Config `mapstructure:"clients"`
}
