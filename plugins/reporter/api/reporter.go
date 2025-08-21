package api

import (
	"reflect"

	"firestige.xyz/otus/internal/otus/msg"
	"firestige.xyz/otus/internal/plugin"
)

// TODO

type Reporter interface {
	plugin.Plugin
	PostConstruct(connection interface{}) error
	Report() error
	ReportType()
}

type ReporterFunc func(batch msg.BatchData) error

func GetReporter(cfg plugin.Config) Reporter {
	return plugin.Get(reflect.TypeOf((*Reporter)(nil)).Elem(), cfg).(Reporter)
}
