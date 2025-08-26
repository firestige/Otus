package api

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
)

type Filter interface {
	plugin.Plugin
	Filter(data []byte) ([]byte, error)
}

func GetFilter(cfg plugin.Config) Filter {
	return plugin.Get(reflect.TypeOf((*Filter)(nil)).Elem(), cfg).(Filter)
}
