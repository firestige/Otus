package factory

import (
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/config"
)

var registry = make(map[otus.ComponentType]map[string]func(cfg interface{}) interface{})

func Register(cType otus.ComponentType, name string, constructor func(cfg interface{}) interface{}) {
	registry[cType][name] = constructor
}

func GetSource(cfg config.SourceConfig) otus.Source {
	factory := registry[otus.ComponentTypeSource][cfg.Name]
	return factory(cfg).(otus.Source)
}

func GetDecoder(cfg config.DecoderConfig) otus.Decoder {
	factory := registry[otus.ComponentTypeDecoder][cfg.Name]
	return factory(cfg).(otus.Decoder)
}

func GetProcessor(cfg config.ProcessorConfig) otus.Processor {
	factory := registry[otus.ComponentTypeProcessor][cfg.Name]
	return factory(cfg).(otus.Processor)
}

func GetSinks(cfg []config.SinkConfig) []otus.Sink {
	sinks := make([]otus.Sink, 0)
	for _, sinkCfg := range cfg {
		factory := registry[otus.ComponentTypeSink][sinkCfg.Name]
		sink := factory(sinkCfg).(otus.Sink)
		sinks = append(sinks, sink)
	}
	return sinks
}
