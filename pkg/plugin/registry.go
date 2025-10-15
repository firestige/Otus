package plugin

import "fmt"

type Registry interface {
	Register(p Plugin) error
	Get(name string) (Plugin, error)
	List(pluginType string) []Plugin
}

var globalRegistry Registry

func SetRegistry(r Registry) {
	globalRegistry = r
}

func Register(p Plugin) error {
	if globalRegistry == nil {
		return fmt.Errorf("registry not initialized")
	}
	return globalRegistry.Register(p)
}

func Get(name string) (Plugin, error) {
	if globalRegistry == nil {
		return nil, fmt.Errorf("registry not initialized")
	}
	return globalRegistry.Get(name)
}

func List(pluginType string) []Plugin {
	if globalRegistry == nil {
		return nil
	}
	return globalRegistry.List(pluginType)
}
