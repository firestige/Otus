package api

import (
	"reflect"

	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/plugin"
)

type Parser interface {
	codec.Parser
	plugin.Plugin
}

func GetParser(cfg plugin.Config) Parser {
	return plugin.Get(reflect.TypeOf((*Parser)(nil)).Elem(), cfg).(Parser)
}
