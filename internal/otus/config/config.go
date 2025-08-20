package config

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/internal/otus/metrics"
	"firestige.xyz/otus/internal/otus/module/capture"
	sender "firestige.xyz/otus/internal/otus/module/sender/api"
	"firestige.xyz/otus/internal/plugin"
	processor "firestige.xyz/otus/internal/processor/api"
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
	Processor    *processor.Config    `mapstructure:"processor"`
	Sender       *sender.Config       `mapstructure:"sender"`
}
