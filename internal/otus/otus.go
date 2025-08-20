package otus

import (
	"context"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"

	"firestige.xyz/otus/internal/plugin"
)

var (
	otus *AppContext
	once sync.Once
)

func GetAppContext() *AppContext {
	once.Do(func() {
		otus = newAppContext()
	})
	return otus
}

type AppContext struct {
	ctx      context.Context
	registry map[reflect.Type]map[string]reflect.Value
}

func newAppContext() *AppContext {
	ctx, cancel := context.WithCancel(context.Background())
	initShutdownListener(cancel)
	return &AppContext{
		ctx:      ctx,
		registry: make(map[reflect.Type]map[string]reflect.Value),
	}
}

func (a *AppContext) GetContext() context.Context {
	return a.ctx
}

func (a *AppContext) GetPlugins(pluginType reflect.Type) []reflect.Value {
	if moduleMap, exists := a.registry[pluginType]; exists {
		var plugins []reflect.Value
		for _, plugin := range moduleMap {
			plugins = append(plugins, plugin)
		}
		return plugins
	}
	return nil
}

// GetPluginByName 根据类型和名称获取特定插件
func (a *AppContext) GetPluginByName(pluginType reflect.Type, name string) reflect.Value {
	if moduleMap, exists := a.registry[pluginType]; exists {
		if plugin, exists := moduleMap[name]; exists {
			return plugin
		}
	}
	return reflect.Value{}
}

func (a *AppContext) SeekAndRegisterModules() {
	a.MergeRegistry(plugin.GetRegistedPlugins())
}

func (a *AppContext) BuildComponents() {

}

func (a *AppContext) StartComponents() error {
	return nil // TODO: Implement component start logic
}

func (a *AppContext) Shutdown() {
}

func initShutdownListener(cancel context.CancelFunc) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		cancel()
	}()
}

func (a *AppContext) MergeRegistry(other map[reflect.Type]map[string]reflect.Value) {
	newRegistry := make(map[reflect.Type]map[string]reflect.Value)

	// Copy from registry
	for t, m := range a.registry {
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
	a.registry = newRegistry
}
