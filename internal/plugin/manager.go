package plugin

import (
	"fmt"
	"reflect"
	"strings"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var registry map[reflect.Type]map[string]reflect.Value

func init() {
	registry = make(map[reflect.Type]map[string]reflect.Value)
}

func RegisterPlugin(plugin Plugin) {
	v := reflect.ValueOf(plugin)
	log.GetLogger().Infof("register plugin: %s", plugin.Name())
	success := false

	for pCategory, pReg := range registry {
		if v.Type().Implements(pCategory) {
			log.GetLogger().Infof("pcategory: %s, pReg is %+v", pCategory.String(), pReg)
			pReg[plugin.Name()] = v
			log.GetLogger().WithFields(logrus.Fields{
				"category":    v.Type().String(),
				"plugin_name": plugin.Name(),
			}).Debug("register plugin success")
			success = true
		}
	}

	if !success {
		log.GetLogger().WithFields(logrus.Fields{
			"category":    v.Type().String(),
			"plugin_name": plugin.Name(),
		}).Error("plugin is not allowed to register")
	}
}

func RegisterPluginType(pluginType reflect.Type) {
	registry[pluginType] = make(map[string]reflect.Value)
}

func GetRegistedPlugins() map[reflect.Type]map[string]reflect.Value {
	return registry
}

func Get(pluginType reflect.Type, cfg Config) Plugin {
	pluginName := findName(cfg)
	value, ok := registry[pluginType][pluginName]
	if !ok {
		panic(fmt.Errorf("cannot find plugin %s of type %s", pluginName, pluginType.Name()))
	}
	t := value.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	plugin := reflect.New(t).Interface().(Plugin)
	Initializing(plugin, cfg)
	return plugin
}

func findName(cfg Config) string {
	name, ok := cfg[NameField]
	if !ok {
		panic(fmt.Errorf("the config must have a field named %s", NameField))
	}
	return name.(string)
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
