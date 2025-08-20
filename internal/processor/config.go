package processor

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/plugin"
)

type Config struct {
	*config.CommonFields
	FilterConfig []plugin.Config `mapstructure:"filters"`
}
