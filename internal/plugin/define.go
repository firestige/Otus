package plugin

const NameField = "plugin_name"

type Plugin interface {
	Name() string
	DefaultConfig() string
}

type SharablePlugin interface {
	Plugin
	PostConstruct() error
	Start() error
	Close() error
}

type Config map[string]interface{}
