package config

import (
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/metrics"
	"firestige.xyz/otus/internal/otus/module/pipeline"
)

type OtusConfig struct {
	Logger  *log.LoggerConfig  `mapstructure:"log"`
	Pipes   []*pipeline.Config `mapstructure:"pipes"`
	Metrics *metrics.Config    `mapstructure:"metrics"`
}
