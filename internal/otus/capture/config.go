package capture

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/otus/capture/codec"
	"firestige.xyz/otus/internal/otus/capture/sniffer"
)

type Config struct {
	*config.CommonFields

	SnifferConfig *sniffer.Options `mapstructure:"sniffer"`
	CodecConfig   *codec.Options   `mapstructure:"codec"`
	WorkerCount   int              `mapstructure:"worker_count"`
}
