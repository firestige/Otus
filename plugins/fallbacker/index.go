package fallbacker

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugins/fallbacker/api"
	"firestige.xyz/otus/plugins/fallbacker/none"
)

func RegisterExtendedFallbackerModule() {
	plugin.RegisterPluginType(reflect.TypeOf((*api.Fallbacker)(nil)).Elem())
	fallbackers := []api.Fallbacker{
		new(none.Fallbacker),
	}
	for _, f := range fallbackers {
		plugin.RegisterPlugin(f)
	}
}
