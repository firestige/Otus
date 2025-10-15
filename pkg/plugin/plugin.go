package plugin

type Metadata struct {
	Name         string   `mapstructure:"plugin_name"`
	Type         string   `mapstructure:"plugin_type"`
	Version      string   `mapstructure:"plugin_version"`
	Description  string   `mapstructure:"plugin_description"`
	Dependencies []string `mapstructure:"plugin_dependencies"`
}

type Plugin interface {
	Metadata() Metadata
	Init(config map[string]interface{}) error
	Start() error
	Stop() error
	Health() error
}
