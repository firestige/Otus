package config

import (
	"context"

	"github.com/mitchellh/mapstructure"
)

const (
	CommonFieldsName = "CommonFields"
	TagName          = "mapstructure"
)

type CommonFields struct {
	PipeName string `mapstructure:"pipe_name"`
}

type Configurable interface {
	ConfigSpec() interface{}
	PostConfig(config interface{}, ctx context.Context) error
}

var registry = make(map[string]func() interface{})

func Register(name string, factory func() interface{}) {
	if _, exists := registry[name]; exists {
		panic("component already registered: " + name)
	}
	registry[name] = factory
}

func BuildComponet(name string, configData map[string]interface{}, ctx context.Context) (interface{}, error) {
	comp := registry[name]()
	configPointer := comp.(Configurable).ConfigSpec()

	err := mapstructure.Decode(configData, configPointer)
	if err != nil {
		return nil, err
	}

	err = comp.(Configurable).PostConfig(configPointer, ctx)
	if err != nil {
		return nil, err
	}
	return comp, nil
}
