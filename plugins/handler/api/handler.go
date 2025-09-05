package api

import (
	"reflect"

	processor "firestige.xyz/otus/internal/otus/module/processor/api"
	"firestige.xyz/otus/internal/plugin"
)

type Handler interface {
	plugin.Plugin
	Handle(exchange *processor.Exchange)
	PostConstruct() error
}

func GetHandler(cfg plugin.Config) Handler {
	return plugin.Get(reflect.TypeOf((*Handler)(nil)).Elem(), cfg).(Handler)
}
