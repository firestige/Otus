package api

import (
	"reflect"

	"firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/plugin"
	reporter "firestige.xyz/otus/plugins/reporter/api"
)

type Fallbacker interface {
	plugin.Plugin
	Fallback(data *api.OutputPacketContext, reporter reporter.ReporterFunc) bool
}

func GetFallbacker(cfg plugin.Config) Fallbacker {
	return plugin.Get(reflect.TypeOf((*Fallbacker)(nil)).Elem(), cfg).(Fallbacker)
}
