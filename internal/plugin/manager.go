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
	pluginType := reflect.TypeOf(plugin).Elem() // 获取接口类型
	successFlag := false

	// 遍历所有已注册的类型，找到匹配的接口
	for registeredType, moduleMap := range registry {
		if reflect.TypeOf(plugin).Implements(registeredType) {
			moduleMap[plugin.Name()] = reflect.ValueOf(plugin)
			log.GetLogger().WithField("category", registeredType.Name()).WithField("module", plugin.Name()).Debug("Registered module")
			successFlag = true
			break
		}
	}

	if !successFlag {
		log.GetLogger().WithField("category", pluginType.Name()).WithField("module", plugin.Name()).Error("Module register failed")
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

// GetPluginsByType 根据类型获取所有插件
func GetPluginsByType(pluginType reflect.Type) []reflect.Value {
	if moduleMap, exists := registry[pluginType]; exists {
		var plugins []reflect.Value
		for _, plugin := range moduleMap {
			plugins = append(plugins, plugin)
		}
		return plugins
	}
	return nil
}

// GetPluginByName 根据类型和名称获取特定插件
func GetPluginByName(pluginType reflect.Type, name string) reflect.Value {
	if moduleMap, exists := registry[pluginType]; exists {
		if plugin, exists := moduleMap[name]; exists {
			return plugin
		}
	}
	return reflect.Value{}
}
