package api

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
)

const (
	_ ClientStatus = iota
	Connected
	Disconnect
)

type ClientStatus int8

type Client interface {
	plugin.SharablePlugin
	GetConnectedClient() interface{}
	RegisterListener(chan<- ClientStatus)
}

func GetClient(config plugin.Config) Client {
	return plugin.Get(reflect.TypeOf((*Client)(nil)).Elem(), config).(Client)
}
