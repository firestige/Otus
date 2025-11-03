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

type ProcessorConfig struct {
	CommonFields `mapstructure:",squash"`
}

type SinkConfig struct {
	CommonFields `mapstructure:",squash"`
}
