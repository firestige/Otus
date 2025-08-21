package reporter

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugins/reporter/api"
)

func RegisterExtendedReporterModule() {
	// Register the extended protocol codec module
	plugin.RegisterPluginType(reflect.TypeOf((*api.Reporter)(nil)).Elem())
	codecs := []api.Reporter{}
	for _, c := range codecs {
		plugin.RegisterPlugin(c)
	}
}
