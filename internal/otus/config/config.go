package config

type CommonFields struct {
	Name string `mapstructure:"name"`
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

type SinksConfig struct {
	Sinks []CommonFields `mapstructure:"sinks"`
}
