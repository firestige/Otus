package capture

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/otus/module/capture/sniffer"
)

type Config struct {
	*config.CommonFields

	SnifferConfig *sniffer.Options `mapstructure:"sniffer"`
	CodecConfig   *codec.Options   `mapstructure:"codec"`
	WorkerCount   int              `mapstructure:"worker_count"`
}
