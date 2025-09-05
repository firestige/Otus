package filter

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugins/handler/api"
)

func RegisterExtendedFilterModule() {
	plugin.RegisterPluginType(reflect.TypeOf((*api.Handler)(nil)).Elem())
	filters := []api.Handler{}
	for _, filter := range filters {
		plugin.RegisterPlugin(filter)
	}
}
