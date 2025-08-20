package config

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/capture"
	"firestige.xyz/otus/internal/otus/metrics"
	"firestige.xyz/otus/internal/otus/sender"
	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/internal/processor"
)

type OtusConfig struct {
	Logger  *log.LoggerConfig `mapstructure:"log"`
	Global  *GlobalConfig     `mapstructure:"global"`
	Pipes   []*PipeConfig     `mapstructure:"pipes"`
	Metrics *metrics.Config   `mapstructure:"metrics"`
}

type GlobalConfig struct {
	Capture *capture.Config `mapstructure:"capture"`
	Clients []plugin.Config `mapstructure:"clients"`
}

type PipeConfig struct {
	CommonConfig *config.CommonFields `mapstructure:"common_config"`
	Capture      *capture.Config      `mapstructure:"capture"`
	Processors   []*processor.Config  `mapstructure:"processors"`
	Sender       *sender.Config       `mapstructure:"sender"`
}
