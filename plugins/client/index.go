package client

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugins/client/api"
	"firestige.xyz/otus/plugins/client/stub"
)

func RegisterExtendedClientModule() {
	plugin.RegisterPluginType(reflect.TypeOf((*api.Client)(nil)).Elem())
	clients := []api.Client{
		new(stub.StubClient),
	}
	for _, client := range clients {
		plugin.RegisterPlugin(client)
	}
}
