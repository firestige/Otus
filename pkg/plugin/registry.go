// Package plugin provides the global plugin registry.
package plugin

import (
	"fmt"
	"sort"

	"firestige.xyz/otus/internal/core"
)

// Factory types - zero-parameter functions that return empty plugin instances.
// Configuration injection happens later via Init().
type (
	CapturerFactory  func() Capturer
	ParserFactory    func() Parser
	ProcessorFactory func() Processor
	ReporterFactory  func() Reporter
)

// Global registry maps - populated during init() phase, read-only at runtime.
var (
	capturerRegistry  = make(map[string]CapturerFactory)
	parserRegistry    = make(map[string]ParserFactory)
	processorRegistry = make(map[string]ProcessorFactory)
	reporterRegistry  = make(map[string]ReporterFactory)
)

// RegisterCapturer registers a capturer factory by name.
// Panics if the name is already registered (indicates a compile-time bug).
func RegisterCapturer(name string, factory CapturerFactory) {
	if name == "" {
		panic("plugin: capturer name cannot be empty")
	}
	if factory == nil {
		panic("plugin: capturer factory cannot be nil")
	}
	if _, exists := capturerRegistry[name]; exists {
		panic(fmt.Sprintf("plugin: capturer %q already registered", name))
	}
	capturerRegistry[name] = factory
}

// RegisterParser registers a parser factory by name.
// Panics if the name is already registered (indicates a compile-time bug).
func RegisterParser(name string, factory ParserFactory) {
	if name == "" {
		panic("plugin: parser name cannot be empty")
	}
	if factory == nil {
		panic("plugin: parser factory cannot be nil")
	}
	if _, exists := parserRegistry[name]; exists {
		panic(fmt.Sprintf("plugin: parser %q already registered", name))
	}
	parserRegistry[name] = factory
}

// RegisterProcessor registers a processor factory by name.
// Panics if the name is already registered (indicates a compile-time bug).
func RegisterProcessor(name string, factory ProcessorFactory) {
	if name == "" {
		panic("plugin: processor name cannot be empty")
	}
	if factory == nil {
		panic("plugin: processor factory cannot be nil")
	}
	if _, exists := processorRegistry[name]; exists {
		panic(fmt.Sprintf("plugin: processor %q already registered", name))
	}
	processorRegistry[name] = factory
}

// RegisterReporter registers a reporter factory by name.
// Panics if the name is already registered (indicates a compile-time bug).
func RegisterReporter(name string, factory ReporterFactory) {
	if name == "" {
		panic("plugin: reporter name cannot be empty")
	}
	if factory == nil {
		panic("plugin: reporter factory cannot be nil")
	}
	if _, exists := reporterRegistry[name]; exists {
		panic(fmt.Sprintf("plugin: reporter %q already registered", name))
	}
	reporterRegistry[name] = factory
}

// GetCapturerFactory returns the factory for the named capturer.
// Returns core.ErrPluginNotFound if not registered.
func GetCapturerFactory(name string) (CapturerFactory, error) {
	factory, ok := capturerRegistry[name]
	if !ok {
		return nil, fmt.Errorf("capturer %q: %w", name, core.ErrPluginNotFound)
	}
	return factory, nil
}

// GetParserFactory returns the factory for the named parser.
// Returns core.ErrPluginNotFound if not registered.
func GetParserFactory(name string) (ParserFactory, error) {
	factory, ok := parserRegistry[name]
	if !ok {
		return nil, fmt.Errorf("parser %q: %w", name, core.ErrPluginNotFound)
	}
	return factory, nil
}

// GetProcessorFactory returns the factory for the named processor.
// Returns core.ErrPluginNotFound if not registered.
func GetProcessorFactory(name string) (ProcessorFactory, error) {
	factory, ok := processorRegistry[name]
	if !ok {
		return nil, fmt.Errorf("processor %q: %w", name, core.ErrPluginNotFound)
	}
	return factory, nil
}

// GetReporterFactory returns the factory for the named reporter.
// Returns core.ErrPluginNotFound if not registered.
func GetReporterFactory(name string) (ReporterFactory, error) {
	factory, ok := reporterRegistry[name]
	if !ok {
		return nil, fmt.Errorf("reporter %q: %w", name, core.ErrPluginNotFound)
	}
	return factory, nil
}

// ListCapturers returns a sorted list of all registered capturer names.
func ListCapturers() []string {
	names := make([]string, 0, len(capturerRegistry))
	for name := range capturerRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListParsers returns a sorted list of all registered parser names.
func ListParsers() []string {
	names := make([]string, 0, len(parserRegistry))
	for name := range parserRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListProcessors returns a sorted list of all registered processor names.
func ListProcessors() []string {
	names := make([]string, 0, len(processorRegistry))
	for name := range processorRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListReporters returns a sorted list of all registered reporter names.
func ListReporters() []string {
	names := make([]string, 0, len(reporterRegistry))
	for name := range reporterRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
