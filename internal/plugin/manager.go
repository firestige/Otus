package plugin

import (
	"reflect"

	"firestige.xyz/otus/internal/log"
)

var registry map[reflect.Type]map[string]reflect.Value

func init() {
	registry = make(map[reflect.Type]map[string]reflect.Value)
}

func RegisterPlugin(plugin Plugin) {
	moduleType := reflect.TypeOf(plugin)
	successFlag := false
	if mReg, exists := registry[moduleType]; !exists {
		mReg[plugin.Name()] = reflect.ValueOf(plugin)
		log.GetLogger().WithField("category", moduleType.Name()).WithField("module", plugin.Name()).Debug("Registered module")
		successFlag = true
	}
	if !successFlag {
		log.GetLogger().WithField("category", moduleType.Name()).WithField("module", plugin.Name()).Error("Module register failed")
	}
}

func RegisterPluginType(pluginType reflect.Type) {
	registry[pluginType] = make(map[string]reflect.Value)
}

func MergeRegistry(other map[reflect.Type]map[string]reflect.Value) map[reflect.Type]map[string]reflect.Value {
	newRegistry := make(map[reflect.Type]map[string]reflect.Value)

	// Copy from registry
	for t, m := range registry {
		newRegistry[t] = make(map[string]reflect.Value)
		for name, val := range m {
			newRegistry[t][name] = val
		}
	}

	// Merge from other
	for t, m := range other {
		if _, exists := newRegistry[t]; !exists {
			newRegistry[t] = make(map[string]reflect.Value)
		}
		for name, val := range m {
			newRegistry[t][name] = val
		}
	}

	return newRegistry
}
