package api

import (
	"reflect"

	"firestige.xyz/otus/internal/otus/msg"
	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugins/reporter/api"
)

type Fallbacker interface {
	plugin.Plugin
	Fallback(data *msg.OutputMessage, reporter api.ReporterFunc) bool
}

func GetFallbacker(cfg plugin.Config) Fallbacker {
	return plugin.Get(reflect.TypeOf((*Fallbacker)(nil)).Elem(), cfg).(Fallbacker)
}
