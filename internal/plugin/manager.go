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

func GetRegistedPlugins() map[reflect.Type]map[string]reflect.Value {
	return registry
}
