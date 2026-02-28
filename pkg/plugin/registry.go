// Package plugin provides the global plugin registry.
package plugin

import (
	"fmt"
	"sort"

	"icc.tech/capture-agent/internal/core"
)

// Registry is a type-safe generic plugin registry that stores factory functions.
// It eliminates duplicated register/get/list logic across plugin types.
// T must be a plugin interface type (Capturer, Parser, Processor, Reporter).
type Registry[T any] struct {
	factories map[string]func() T
	typeName  string // e.g. "capturer", for error messages
}

// NewRegistry creates a new generic plugin registry.
func NewRegistry[T any](typeName string) *Registry[T] {
	return &Registry[T]{
		factories: make(map[string]func() T),
		typeName:  typeName,
	}
}

// Register registers a factory function by name.
// Panics if name is empty, factory is nil, or name is already registered.
func (r *Registry[T]) Register(name string, factory func() T) {
	if name == "" {
		panic(fmt.Sprintf("plugin: %s name cannot be empty", r.typeName))
	}
	if factory == nil {
		panic(fmt.Sprintf("plugin: %s factory cannot be nil", r.typeName))
	}
	if _, exists := r.factories[name]; exists {
		panic(fmt.Sprintf("plugin: %s %q already registered", r.typeName, name))
	}
	r.factories[name] = factory
}

// Get returns the factory for the named plugin.
// Returns core.ErrPluginNotFound if not registered.
func (r *Registry[T]) Get(name string) (func() T, error) {
	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("%s %q: %w", r.typeName, name, core.ErrPluginNotFound)
	}
	return factory, nil
}

// List returns a sorted list of all registered plugin names.
func (r *Registry[T]) List() []string {
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Reset clears the registry. Intended for testing only.
func (r *Registry[T]) Reset() {
	r.factories = make(map[string]func() T)
}

// Factory types - zero-parameter functions that return empty plugin instances.
// Configuration injection happens later via Init().
type (
	CapturerFactory  func() Capturer
	ParserFactory    func() Parser
	ProcessorFactory func() Processor
	ReporterFactory  func() Reporter
)

// Global registry instances — populated during init() phase, read-only at runtime.
var (
	capturerReg  = NewRegistry[Capturer]("capturer")
	parserReg    = NewRegistry[Parser]("parser")
	processorReg = NewRegistry[Processor]("processor")
	reporterReg  = NewRegistry[Reporter]("reporter")
)

// ──── Capturer ────

// RegisterCapturer registers a capturer factory by name.
func RegisterCapturer(name string, factory CapturerFactory) {
	capturerReg.Register(name, factory)
}

// GetCapturerFactory returns the factory for the named capturer.
func GetCapturerFactory(name string) (CapturerFactory, error) {
	return capturerReg.Get(name)
}

// ListCapturers returns a sorted list of all registered capturer names.
func ListCapturers() []string {
	return capturerReg.List()
}

// ──── Parser ────

// RegisterParser registers a parser factory by name.
func RegisterParser(name string, factory ParserFactory) {
	parserReg.Register(name, factory)
}

// GetParserFactory returns the factory for the named parser.
func GetParserFactory(name string) (ParserFactory, error) {
	return parserReg.Get(name)
}

// ListParsers returns a sorted list of all registered parser names.
func ListParsers() []string {
	return parserReg.List()
}

// ──── Processor ────

// RegisterProcessor registers a processor factory by name.
func RegisterProcessor(name string, factory ProcessorFactory) {
	processorReg.Register(name, factory)
}

// GetProcessorFactory returns the factory for the named processor.
func GetProcessorFactory(name string) (ProcessorFactory, error) {
	return processorReg.Get(name)
}

// ListProcessors returns a sorted list of all registered processor names.
func ListProcessors() []string {
	return processorReg.List()
}

// ──── Reporter ────

// RegisterReporter registers a reporter factory by name.
func RegisterReporter(name string, factory ReporterFactory) {
	reporterReg.Register(name, factory)
}

// GetReporterFactory returns the factory for the named reporter.
func GetReporterFactory(name string) (ReporterFactory, error) {
	return reporterReg.Get(name)
}

// ListReporters returns a sorted list of all registered reporter names.
func ListReporters() []string {
	return reporterReg.List()
}
