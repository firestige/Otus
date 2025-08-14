package config

import (
	"context"

	"firestige.xyz/otus/internal/log"
	"github.com/mitchellh/mapstructure"
)

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

type OtusConfig struct {
	Logger *log.LoggerConfig `mapstructure:"log"`
}
