package client

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugins/client/api"
)

func RegisterExtendedClientModule() {
	plugin.RegisterPluginType(reflect.TypeOf((*api.Client)(nil)).Elem())
	clients := []api.Client{}
	for _, client := range clients {
		plugin.RegisterPlugin(client)
	}
}
