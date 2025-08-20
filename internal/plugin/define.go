package plugin

const NameField = "plugin_name"

type Plugin interface {
	Name() string
}

type SharablePlugin interface {
	Plugin
	Start() error
	Stop() error
}

type Config map[string]interface{}
