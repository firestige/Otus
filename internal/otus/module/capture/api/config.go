package api

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/otus/module/capture/handle"
	"firestige.xyz/otus/internal/plugin"
)

type Config struct {
	*config.CommonFields

	HandleConfig *handle.Options `mapstructure:"handle"`
	CodecConfig  *codec.Options  `mapstructure:"codec"`
	ParserConfig []plugin.Config `mapstructure:"parsers"`
	FanoutID     uint16          `mapstructure:"fanout_id"`
}
