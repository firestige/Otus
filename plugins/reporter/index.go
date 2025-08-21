package reporter

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
)

type Reporter interface {
	plugin.SharablePlugin
}

func RegisterExtendedReporterModule() {
	// Register the extended protocol codec module
	plugin.RegisterPluginType(reflect.TypeOf((*Reporter)(nil)).Elem())
	codecs := []Reporter{}
	for _, c := range codecs {
		plugin.RegisterPlugin(c)
	}
}
