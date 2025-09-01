package api

import (
	"reflect"

	"firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/plugin"
)

// TODO

type Reporter interface {
	plugin.Plugin
	PostConstruct() error
	Report(batch api.BatchPacket) error
	SupportProtocol() string
}

type ReporterFunc func(batch api.BatchPacket) error

func GetReporter(cfg plugin.Config) Reporter {
	return plugin.Get(reflect.TypeOf((*Reporter)(nil)).Elem(), cfg).(Reporter)
}
