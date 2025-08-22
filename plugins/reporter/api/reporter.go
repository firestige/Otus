package api

import (
	"reflect"

	"firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/plugin"
)

// TODO

type Reporter interface {
	plugin.Plugin
	PostConstruct(connection interface{}) error
	Report() error
	ReportType()
}

type ReporterFunc func(batch api.BatchePacket) error

func GetReporter(cfg plugin.Config) Reporter {
	return plugin.Get(reflect.TypeOf((*Reporter)(nil)).Elem(), cfg).(Reporter)
}
