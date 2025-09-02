package api

import (
	"reflect"

	"firestige.xyz/otus/internal/otus/event"
	"firestige.xyz/otus/internal/plugin"
)

type Filter interface {
	plugin.Plugin
	PostConstruct() error
	Filter(event *event.EventContext)
}

func GetFilter(cfg plugin.Config) Filter {
	return plugin.Get(reflect.TypeOf((*Filter)(nil)).Elem(), cfg).(Filter)
}
