package plugin

import (
	"fmt"
	"reflect"
	"strings"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	"github.com/spf13/viper"
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

func Get(pluginType reflect.Type, cfg Config) Plugin {
	if moduleMap, exists := registry[pluginType]; exists {
		for _, plugin := range moduleMap {
			if p, ok := plugin.Interface().(Plugin); ok {
				Initializing(p, cfg)
				return p
			}
		}
	}
	log.GetLogger().WithField("category", pluginType.Name()).Error("No suitable module found")
	return nil
}

func GetPluginByName(pluginType reflect.Type, name string) reflect.Value {
	if moduleMap, exists := registry[pluginType]; exists {
		if plugin, exists := moduleMap[name]; exists {
			return plugin
		}
	}
	log.GetLogger().WithField("category", pluginType.Name()).WithField("module", name).Error("Module not found")
	return reflect.Value{}
}

// Initializing initialize the fields by fields mapping.
func Initializing(plugin Plugin, cfg Config) {
	v := viper.New()
	v.SetConfigType("yaml")
	if plugin.DefaultConfig() != "" {
		if err := v.ReadConfig(strings.NewReader(plugin.DefaultConfig())); err != nil {
			panic(fmt.Errorf("cannot read default config in the plugin: %s, the error is %v", plugin.Name(), err))
		}
	}
	if err := v.MergeConfigMap(cfg); err != nil {
		panic(fmt.Errorf("%s plugin cannot merge the custom configuration, the error is %v", plugin.Name(), err))
	}
	if err := v.Unmarshal(plugin); err != nil {
		panic(fmt.Errorf("cannot inject  the config to the %s plugin, the error is %v", plugin.Name(), err))
	}
	cf := reflect.ValueOf(plugin).Elem().FieldByName(config.CommonFieldsName)
	if !cf.IsValid() {
		panic(fmt.Errorf("%s plugin must have a field named CommonField", plugin.Name()))
	}
	for i := 0; i < cf.NumField(); i++ {
		tagVal := cf.Type().Field(i).Tag.Get(config.TagName)
		if tagVal != "" {
			if val := cfg[strings.ToLower(config.CommonFieldsName)+"_"+tagVal]; val != nil {
				cf.Field(i).Set(reflect.ValueOf(val))
			}
		}
	}
}
