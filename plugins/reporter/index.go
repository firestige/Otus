package reporter

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugins/reporter/api"
	"firestige.xyz/otus/plugins/reporter/consolelog"
)

func RegisterExtendedReporterModule() {
	// Register the extended protocol codec module
	plugin.RegisterPluginType(reflect.TypeOf((*api.Reporter)(nil)).Elem())
	codecs := []api.Reporter{
		new(consolelog.Console),
	}
	for _, c := range codecs {
		plugin.RegisterPlugin(c)
	}
}
