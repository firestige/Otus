package filter

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugins/filter/api"
)

func RegisterExtendedFilterModule() {
	plugin.RegisterPluginType(reflect.TypeOf((*api.Filter)(nil)).Elem())
	filters := []api.Filter{}
	for _, filter := range filters {
		plugin.RegisterPlugin(filter)
	}
}
