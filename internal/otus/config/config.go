package config

type CommonFields struct {
	Name string `mapstructure:"name"`
}

func (cf *CommonFields) GetName() string {
	return cf.Name
}

type SourceConfig struct {
	CommonFields `mapstructure:",squash"`
}

type DecoderConfig struct {
	CommonFields `mapstructure:",squash"`
}

type FiltersConfig struct {
	Filters []CommonFields `mapstructure:"filters"`
}

type FilterConfig struct {
	CommonFields `mapstructure:",squash"`
}

type SinksConfig struct {
	Sinks []CommonFields `mapstructure:"sinks"`
}

type SinkConfig struct {
	CommonFields `mapstructure:",squash"`
}
