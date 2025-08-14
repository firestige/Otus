package sender

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
)

type Sender interface {
	plugin.SharablePlugin
}

func RegisterExtendedSenderModule() {
	// Register the extended protocol codec module
	plugin.RegisterPluginType(reflect.TypeOf((*Sender)(nil)).Elem())
	codecs := []Sender{}
	for _, c := range codecs {
		plugin.RegisterPlugin(c)
	}
}
