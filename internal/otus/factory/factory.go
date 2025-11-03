package factory

import (
	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/config"
	"firestige.xyz/otus/internal/processor"
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

func GetProcessor(filters []otus.Filter, sinks []otus.Sink) otus.Processor {
	return processor.NewProcessor(filters, sinks)
}

func GetFilters(cfg config.FiltersConfig) []otus.Filter {
	filters := make([]otus.Filter, 0)
	for _, filterCfg := range cfg.Filters {
		factory := registry[otus.ComponentTypeFilter][filterCfg.Name]
		filter := factory(filterCfg).(otus.Filter)
		filters = append(filters, filter)
	}
	return filters
}

func GetSinks(cfg config.SinksConfig) []otus.Sink {
	sinks := make([]otus.Sink, 0)
	for _, sinkCfg := range cfg.Sinks {
		factory := registry[otus.ComponentTypeSink][sinkCfg.Name]
		sink := factory(sinkCfg).(otus.Sink)
		sinks = append(sinks, sink)
	}
	return sinks
}
