package plugin

import (
	"context"
	"fmt"
	"sync"
	"time"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/pkg/plugin"
)

type PluginState int

const (
	StateRegistered  PluginState = iota // Plugin has been registered but not yet initialized
	StateInitialized                    // Plugin has been initialized and is ready to use
	StateReady                          // Plugin has been started and is active
	StateStopped                        // Plugin has been stopped and is no longer active
	StateError                          // Plugin encountered an error during its lifecycle
)

func (ps PluginState) String() string {
	return [...]string{"Registered", "Initialized", "Ready", "Stopped", "Error"}[ps]
}

type PluginStatus struct {
	Name  string      // Name of the plugin
	Type  string      // Type of the plugin (e.g., "gatherer", "forwarder", etc.)
	State PluginState // Current state of the plugin
	Error error       // Error encountered during plugin lifecycle, if any
}

type ManagerConfig struct {
	InitTimeout  time.Duration // Timeout for plugin initialization
	StartTimeout time.Duration // Timeout for starting plugins
	StopTimeout  time.Duration // Timeout for stopping plugins

	HealthCheckInterval time.Duration // Interval for health checks
	HealthCheckTimeout  time.Duration // Timeout for health checks
}

type Manager struct {
	config   ManagerConfig
	registry *registryImpl

	mu       sync.RWMutex
	statuses map[string]*PluginStatus

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewManager(config ManagerConfig, registry *registryImpl) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		config:   config,
		registry: registry,
		statuses: make(map[string]*PluginStatus),
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (m *Manager) Initialize(configs map[string]map[string]interface{}) error {
	order, err := m.registry.GetLoadOrder()
	if err != nil {
		return err
	}

	log.GetLogger().Info("Initializing plugins in dependency order:")

	for _, name := range order {
		p, _ := m.registry.Get(name)
		meta := p.Metadata()
		log.GetLogger().Infof(" - %s (%s)", name, meta.Type)
		config := configs[name]
		if config == nil {
			config = make(map[string]interface{})
		}
		if err := m.initPlugin(p, config); err != nil {
			log.GetLogger().Errorf("Failed to initialize plugin %s: %v", name, err)
			return err
		}
		log.GetLogger().Infof("Initialized plugin %s", name)
	}

	return nil
}

func (m *Manager) initPlugin(p plugin.Plugin, config map[string]interface{}) error {
	meta := p.Metadata()

	m.mu.Lock()
	m.statuses[meta.Name] = &PluginStatus{
		Name:  meta.Name,
		Type:  meta.Type,
		State: StateRegistered,
	}
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(m.ctx, m.config.InitTimeout)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- p.Init(config)
	}()

	select {
	case <-ctx.Done():
		err := fmt.Errorf("initialization timeout after %v", m.config.InitTimeout)
		m.updateStatus(meta.Name, StateError, err)
		return err
	case err := <-errChan:
		if err != nil {
			m.updateStatus(meta.Name, StateError, err)
			return err
		}
		m.updateStatus(meta.Name, StateInitialized, nil)
		return nil
	}
}

func (m *Manager) Start() error {
	order, err := m.registry.GetLoadOrder()
	if err != nil {
		return err
	}

	log.GetLogger().Info("Starting plugins:")

	for _, name := range order {
		p, _ := m.registry.Get(name)

		if err := m.startPlugin(p); err != nil {
			log.GetLogger().Errorf("Failed to start plugin %s: %v", name, err)
			return err
		}
		log.GetLogger().Infof("Started plugin %s", name)
	}

	if m.config.HealthCheckInterval > 0 {
		m.wg.Add(1)
		go m.healthCheckLoop()
	}

	return nil
}

func (m *Manager) startPlugin(p plugin.Plugin) error {
	meta := p.Metadata()

	ctx, cancel := context.WithTimeout(m.ctx, m.config.StartTimeout)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- p.Start()
	}()

	select {
	case <-ctx.Done():
		err := fmt.Errorf("start timeout after %v", m.config.StartTimeout)
		m.updateStatus(meta.Name, StateError, err)
		return err
	case err := <-errChan:
		if err != nil {
			m.updateStatus(meta.Name, StateError, err)
			return err
		}
		m.updateStatus(meta.Name, StateReady, nil)
		return nil
	}
}

func (m *Manager) Stop() error {
	m.cancel()
	m.wg.Wait()

	order, err := m.registry.GetLoadOrder()
	if err != nil {
		return err
	}

	log.GetLogger().Info("Stopping plugins:")

	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		p, _ := m.registry.Get(name)

		log.GetLogger().Infof(" - Stopping plugin %s", name)

		if err := m.stopPlugin(p); err != nil {
			log.GetLogger().Errorf("Failed to stop plugin %s: %v", name, err)
			return err
		} else {
			log.GetLogger().Infof("Stopped plugin %s", name)
		}
	}

	return nil
}

func (m *Manager) stopPlugin(p plugin.Plugin) error {
	meta := p.Metadata()

	ctx, cancel := context.WithTimeout(context.Background(), m.config.StopTimeout)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- p.Stop()
	}()

	select {
	case <-ctx.Done():
		err := fmt.Errorf("stop timeout after %v", m.config.StopTimeout)
		m.updateStatus(meta.Name, StateError, err)
		return err
	case err := <-errChan:
		if err != nil {
			m.updateStatus(meta.Name, StateError, err)
			return err
		}
		m.updateStatus(meta.Name, StateStopped, nil)
		return nil
	}
}

func (m *Manager) healthCheckLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAllPlugins()
		}
	}
}

func (m *Manager) checkAllPlugins() {
	m.mu.RLock()
	readyPlugins := make([]string, 0)
	for name, status := range m.statuses {
		if status.State == StateReady {
			readyPlugins = append(readyPlugins, name)
		}
	}
	m.mu.RUnlock()

	for _, name := range readyPlugins {
		p, _ := m.registry.Get(name)

		ctx, cancel := context.WithTimeout(m.ctx, m.config.HealthCheckTimeout)
		errChan := make(chan error, 1)

		go func() {
			errChan <- p.Health()
		}()

		select {
		case <-ctx.Done():
			err := fmt.Errorf("health check timeout after %v", m.config.HealthCheckTimeout)
			m.updateStatus(name, StateError, err)
			log.GetLogger().Errorf("Health check timeout for plugin %s: %v", name, err)
		case err := <-errChan:
			if err != nil {
				m.updateStatus(name, StateError, err)
				log.GetLogger().Errorf("Health check failed for plugin %s: %v", name, err)
			}
		}
		cancel()
	}
}

func (m *Manager) GetStatus(name string) (*PluginStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if status, exists := m.statuses[name]; exists {
		return status, nil
	}
	return nil, fmt.Errorf("plugin %s not found", name)
}

func (m *Manager) GetAllStatuses() map[string]*PluginStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make(map[string]*PluginStatus, len(m.statuses))
	for name, status := range m.statuses {
		statuses[name] = status
	}
	return statuses
}

func (m *Manager) updateStatus(name string, state PluginState, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if status, exists := m.statuses[name]; exists {
		status.State = state
		status.Error = err
	}
}
