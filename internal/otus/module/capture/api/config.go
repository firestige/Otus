package api

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/otus/module/capture/sniffer"
	"firestige.xyz/otus/internal/plugin"
)

type Config struct {
	*config.CommonFields

	SnifferConfig *sniffer.Options `mapstructure:"sniffer"`
	CodecConfig   *codec.Options   `mapstructure:"codec"`
	ParserConfig  []plugin.Config  `mapstructure:"parsers"`
	WorkerCount   int              `mapstructure:"worker_count"`
}
