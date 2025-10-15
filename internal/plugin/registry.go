package plugin

import (
	"fmt"
	"slices"
	"sort"
	"sync"

	plugin "firestige.xyz/otus/pkg/plugin"
)

var supportedTypes = []string{"gatherer", "processor", "forwarder"}

type registryImpl struct {
	mu      sync.RWMutex
	plugins map[string]plugin.Plugin // map[pluginName] = plugin
	types   map[string][]string      // map[type] = []pluginNames
}

func NewRegistry() *registryImpl {
	r := &registryImpl{
		plugins: make(map[string]plugin.Plugin),
		types:   make(map[string][]string),
	}
	plugin.SetRegistry(r)
	return r
}

func (r *registryImpl) Register(p plugin.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta := p.Metadata()

	if _, exists := r.plugins[meta.Name]; exists {
		return fmt.Errorf("plugin '%s' already registered", meta.Name)
	}

	if !slices.Contains(supportedTypes, meta.Type) {
		return fmt.Errorf("plugin '%s' has unsupported type '%s'", meta.Name, meta.Type)
	}

	r.plugins[meta.Name] = p
	r.types[meta.Type] = append(r.types[meta.Type], meta.Name)
	return nil
}

func (r *registryImpl) Get(name string) (plugin.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin '%s' not found", name)
	}
	return p, nil
}

func (r *registryImpl) List(pluginType string) []plugin.Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names, exists := r.types[pluginType]
	if !exists {
		return nil
	}

	plugins := make([]plugin.Plugin, 0, len(names))
	for _, name := range names {
		if p, exists := r.plugins[name]; exists {
			plugins = append(plugins, p)
		}
	}
	return plugins
}

func (r *registryImpl) GetLoadOrder() ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	graph := make(map[string][]string) // map[pluginName] = []dependentPluginNames
	inDegree := make(map[string]int)   // map[pluginName] = numberOfDependencies

	for name, p := range r.plugins {
		meta := p.Metadata()
		inDegree[name] = 0

		for _, dep := range meta.Dependencies {
			if _, exists := r.plugins[dep]; !exists {
				return nil, fmt.Errorf("plugin '%s' has unknown dependency '%s'", name, dep)
			}
			graph[dep] = append(graph[dep], name)
		}
	}

	for name, p := range r.plugins {
		meta := p.Metadata()
		inDegree[name] = len(meta.Dependencies)
	}

	queue := make([]string, 0)
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	sort.Strings(queue) // Ensure deterministic order

	result := make([]string, 0, len(r.plugins))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		dependents := graph[current]
		sort.Strings(dependents)

		for _, dep := range dependents {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
				sort.Strings(queue) // Maintain order
			}
		}
	}

	if len(result) != len(r.plugins) {
		return nil, fmt.Errorf("circular dependency detected among plugins")
	}

	return result, nil
}
