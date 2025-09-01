package api

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/plugin"
)

type Config struct {
	*config.CommonFields

	FilterConfigs []plugin.Config `mapstructure:"filters"`
}
