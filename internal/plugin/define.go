package plugin

import "firestige.xyz/otus/internal/config"

type Plugin interface {
	config.Configurable
	Name() string
}

type SharablePlugin interface {
	Plugin
	Start() error
	Stop() error
}
