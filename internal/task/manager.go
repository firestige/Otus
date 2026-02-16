// Package task implements task lifecycle management.
package task

import (
	"fmt"
	"log/slog"
	"sync"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/core/decoder"
	"firestige.xyz/otus/internal/pipeline"
	"firestige.xyz/otus/pkg/plugin"
)

// TaskManager manages task CRUD and state machine.
type TaskManager struct {
	mu    sync.RWMutex
	tasks map[string]*Task // task_id → Task

	// Global configuration
	agentID string
}

// NewTaskManager creates a new task manager.
func NewTaskManager(agentID string) *TaskManager {
	return &TaskManager{
		tasks:   make(map[string]*Task),
		agentID: agentID,
	}
}

// Create creates and starts a new task from configuration.
// This implements the 6-phase assembly process described in architecture.md:
// 1. Validate - check TaskConfig completeness
// 2. Resolve - lookup all factories from Registry, fail fast if any not found
// 3. Construct - call factories to create empty instances
// 4. Init - inject plugin-specific config
// 5. Wire - inject Task-level shared resources (FlowRegistry)
// 6. Assemble & Start - build Task and start in dependency reverse order
func (m *TaskManager) Create(cfg config.TaskConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Phase 1 limitation: maximum 1 task
	if len(m.tasks) >= 1 {
		return fmt.Errorf("phase 1 limitation: maximum 1 task allowed (current: %d)", len(m.tasks))
	}

	// Check for duplicate ID
	if _, exists := m.tasks[cfg.ID]; exists {
		return fmt.Errorf("task %q already exists", cfg.ID)
	}

	slog.Info("creating task", "task_id", cfg.ID)

	// ========== Phase 1: Validate ==========
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// ========== Phase 2: Resolve ==========
	// Lookup all plugin factories before creating any instances (fail-fast).
	slog.Debug("resolving plugins", "task_id", cfg.ID)

	capFactory, err := plugin.GetCapturerFactory(cfg.Capture.Type)
	if err != nil {
		return fmt.Errorf("capturer %q: %w", cfg.Capture.Type, err)
	}

	parserFactories := make([]plugin.ParserFactory, len(cfg.Parsers))
	for i, pc := range cfg.Parsers {
		f, err := plugin.GetParserFactory(pc.Type)
		if err != nil {
			return fmt.Errorf("parser %q: %w", pc.Type, err)
		}
		parserFactories[i] = f
	}

	processorFactories := make([]plugin.ProcessorFactory, len(cfg.Processors))
	for i, pc := range cfg.Processors {
		f, err := plugin.GetProcessorFactory(pc.Type)
		if err != nil {
			return fmt.Errorf("processor %q: %w", pc.Type, err)
		}
		processorFactories[i] = f
	}

	// Reporter: take first one for now (TODO: support multiple reporters)
	var repFactory plugin.ReporterFactory
	if len(cfg.Reporters) > 0 {
		f, err := plugin.GetReporterFactory(cfg.Reporters[0].Type)
		if err != nil {
			return fmt.Errorf("reporter %q: %w", cfg.Reporters[0].Type, err)
		}
		repFactory = f
	}

	// ========== Phase 3: Construct ==========
	slog.Debug("constructing plugin instances", "task_id", cfg.ID)

	// Task-level singleton instances
	task := NewTask(cfg)

	// Capturer: 1 per Task
	task.Capturer = capFactory()

	// Reporter: 1 per Task
	if repFactory != nil {
		task.Reporter = repFactory()
	}

	// FlowRegistry: 1 per Task (shared across pipelines)
	task.Registry = NewFlowRegistry()

	// Decoder: 1 per Task (stateless, shared across pipelines)
	sharedDecoder := decoder.NewStandardDecoder(decoder.Config{
		Tunnels:      cfg.Decoder.Tunnels,
		IPReassembly: cfg.Decoder.IPReassembly,
	})

	// Pipelines: N copies, each with independent Parser/Processor instances
	numPipelines := cfg.Capture.FanoutSize
	if numPipelines < 1 {
		numPipelines = 1
	}

	for i := 0; i < numPipelines; i++ {
		// Each pipeline gets its own Parser instances
		parsers := make([]plugin.Parser, len(cfg.Parsers))
		for j := range cfg.Parsers {
			parsers[j] = parserFactories[j]() // Factory creates empty instance
		}

		// Each pipeline gets its own Processor instances
		processors := make([]plugin.Processor, len(cfg.Processors))
		for j := range cfg.Processors {
			processors[j] = processorFactories[j]()
		}

		// ========== Phase 4: Init (for this pipeline's plugins) ==========
		// Init Parsers
		for parserIdx, parser := range parsers {
			if err := parser.Init(cfg.Parsers[parserIdx].Config); err != nil {
				return fmt.Errorf("pipeline %d parser %d init failed: %w", i, parserIdx, err)
			}
		}

		// Init Processors
		for procIdx, proc := range processors {
			if err := proc.Init(cfg.Processors[procIdx].Config); err != nil {
				return fmt.Errorf("pipeline %d processor %d init failed: %w", i, procIdx, err)
			}
		}

		// ========== Phase 5: Wire (for this pipeline's plugins) ==========
		// Inject FlowRegistry into parsers that need it
		for parserIdx, parser := range parsers {
			if fra, ok := parser.(plugin.FlowRegistryAware); ok {
				fra.SetFlowRegistry(task.Registry)
				slog.Debug("injected FlowRegistry into parser",
					"task_id", cfg.ID,
					"pipeline_id", i,
					"parser_id", parserIdx,
					"parser_type", parser.Name())
			}
		}

		// Create pipeline with fully initialized plugins
		p := pipeline.New(pipeline.Config{
			ID:         i,
			TaskID:     cfg.ID,
			AgentID:    m.agentID,
			Decoder:    sharedDecoder,
			Parsers:    parsers,
			Processors: processors,
		})

		task.Pipelines = append(task.Pipelines, p)
	}

	// ========== Phase 4: Init (Task-level plugins) ==========
	slog.Debug("initializing task-level plugins", "task_id", cfg.ID)

	// Init Capturer
	if err := task.Capturer.Init(cfg.Capture.Extra); err != nil {
		return fmt.Errorf("capturer init failed: %w", err)
	}

	// Init Reporter
	if task.Reporter != nil && len(cfg.Reporters) > 0 {
		if err := task.Reporter.Init(cfg.Reporters[0].Config); err != nil {
			return fmt.Errorf("reporter init failed: %w", err)
		}
	}

	// ========== Phase 6: Assemble & Start ==========
	slog.Debug("starting task", "task_id", cfg.ID)

	// Start task (this will start all components in correct order)
	if err := task.Start(); err != nil {
		return fmt.Errorf("task start failed: %w", err)
	}

	// Register task in manager
	m.tasks[cfg.ID] = task

	slog.Info("task created successfully",
		"task_id", cfg.ID,
		"pipelines", numPipelines,
		"state", task.State())

	return nil
}

// Delete stops and removes a task.
func (m *TaskManager) Delete(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, exists := m.tasks[taskID]
	if !exists {
		return fmt.Errorf("task %q not found", taskID)
	}

	slog.Info("deleting task", "task_id", taskID)

	// Stop task
	if err := task.Stop(); err != nil {
		slog.Warn("error stopping task", "task_id", taskID, "error", err)
		// Continue with deletion even if stop failed
	}

	// Remove from manager
	delete(m.tasks, taskID)

	slog.Info("task deleted", "task_id", taskID)
	return nil
}

// Get retrieves a task by ID.
func (m *TaskManager) Get(taskID string) (*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, exists := m.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task %q not found", taskID)
	}

	return task, nil
}

// List returns a list of all task IDs.
func (m *TaskManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.tasks))
	for id := range m.tasks {
		ids = append(ids, id)
	}

	return ids
}

// Status returns status for all tasks.
func (m *TaskManager) Status() map[string]Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]Status, len(m.tasks))
	for id, task := range m.tasks {
		status[id] = task.GetStatus()
	}

	return status
}

// Count returns the number of active tasks.
func (m *TaskManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.tasks)
}

// StopAll stops all tasks (useful for shutdown).
func (m *TaskManager) StopAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	slog.Info("stopping all tasks", "count", len(m.tasks))

	var lastErr error
	for id, task := range m.tasks {
		if err := task.Stop(); err != nil {
			slog.Warn("error stopping task", "task_id", id, "error", err)
			lastErr = err
		}
	}

	// Clear all tasks
	m.tasks = make(map[string]*Task)

	return lastErr
}

