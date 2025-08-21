package fallbacker

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugins/fallbacker/api"
)

func RegisterExtendedFallbackerModule() {
	plugin.RegisterPluginType(reflect.TypeOf((*api.Fallbacker)(nil)).Elem())
	fallbackers := []api.Fallbacker{}
	for _, f := range fallbackers {
		plugin.RegisterPlugin(f)
	}
}
