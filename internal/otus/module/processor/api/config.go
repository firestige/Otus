package api

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/plugin"
)

type Config struct {
	*config.CommonFields

	HandlerConfigs []plugin.Config `mapstructure:"handlers"`
}
