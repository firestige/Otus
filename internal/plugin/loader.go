package plugin

import (
	"fmt"
	"path/filepath"
	"plugin"
	"strings"
)

type LoadMode string

const (
	DynamicMode LoadMode = "dynamic" // use plugin package to load .so files
	StaticMode  LoadMode = "static"  // use build tags to include plugins at compile time
)

type LoaderConfig struct {
	Mode     LoadMode
	Path     string   // directory to load plugins from in DynamicMode
	Patterns []string // file patterns to match plugins in DynamicMode
}

type Loader struct {
	config   LoaderConfig
	registry *registryImpl
}

func NewLoader(config LoaderConfig, registry *registryImpl) *Loader {
	return &Loader{
		config:   config,
		registry: registry,
	}
}

func (l *Loader) Load() error {
	if l.config.Mode == StaticMode {
		// In static mode, plugins are registered at compile time. we just validate the registry.
		return l.validateStaticPlugins()
	}

	// In dynamic mode, seek for .so files in the specified path and load them.
	return l.loadDynamicPlugins()
}

func (l *Loader) validateStaticPlugins() error {
	// static plugins should already be registered in the registry. We only need to ensure the order.
	_, err := l.registry.GetLoadOrder()
	if err != nil {
		return fmt.Errorf("plugin dependency validation failed: %w", err)
	}
	return nil
}

func (l *Loader) loadDynamicPlugins() error {
	// Implementation for loading dynamic plugins would go here.
	// This typically involves using the "plugin" package to open .so files,
	pluginFiles, err := l.discoverPluginFiles()
	if err != nil {
		return fmt.Errorf("failed to discover plugin files: %w", err)
	}

	if len(pluginFiles) == 0 {
		return fmt.Errorf("no plugin files found in path: %s", l.config.Path)
	}

	for _, file := range pluginFiles {
		if err := l.loadPlugin(file); err != nil {
			return fmt.Errorf("failed to load plugin %s: %w", file, err)
		}
	}

	// After loading all plugins, validate dependencies.
	_, err = l.registry.GetLoadOrder()
	if err != nil {
		return fmt.Errorf("plugin dependency validation failed: %w", err)
	}

	return nil
}

func (l *Loader) discoverPluginFiles() ([]string, error) {
	// This function would implement the logic to discover plugin files
	// in the specified directory matching the given patterns.
	files := make([]string, 0)

	for _, pattern := range l.config.Patterns {
		fullPattern := filepath.Join(l.config.Path, pattern)
		matches, err := filepath.Glob(fullPattern)
		if err != nil {
			return nil, fmt.Errorf("failed to match pattern %s: %w", fullPattern, err)
		}
		files = append(files, matches...)
	}
	return files, nil
}

func (l *Loader) loadPlugin(file string) error {
	p, err := plugin.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open plugin file %s: %w", file, err)
	}

	symbolRegister, err := p.Lookup("Register")
	if err != nil {
		return fmt.Errorf("plugin %s does not export Register function: %w", file, err)
	}

	registerFunc, ok := symbolRegister.(func() error)
	if !ok {
		return fmt.Errorf("plugin %s Register function has invalid signature", file)
	}

	if err := registerFunc(); err != nil {
		return fmt.Errorf("plugin %s registration failed: %w", file, err)
	}

	name := filepath.Base(file)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	fmt.Printf("Successfully loaded plugin: %s\n", name)

	return nil
}
