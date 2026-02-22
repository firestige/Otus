// Package task implements task lifecycle management.
package task

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

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

	// store is the persistence backend (noopStore when disabled).
	store TaskStore
}

// NewTaskManager creates a new task manager.
// store is the persistence backend; pass nil to disable persistence.
func NewTaskManager(agentID string, store TaskStore) *TaskManager {
	if store == nil {
		store = noopStore{}
	}
	return &TaskManager{
		tasks:   make(map[string]*Task),
		agentID: agentID,
		store:   store,
	}
}

// Create creates and starts a new task from configuration.
// This implements the strict 7-phase assembly process described in architecture.md:
// 1. Validate  - check TaskConfig completeness
// 2. Resolve   - lookup all factories from Registry, fail fast if any not found
// 3. Construct - call factories to create all empty instances
// 4. Init      - inject plugin-specific config into all instances
// 5. Wire      - inject Task-level shared resources (FlowRegistry)
// 6. Assemble  - build Pipelines and Task struct
// 7. Start     - start in dependency reverse order
//
// Each phase completes fully before the next begins (strict separation).
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

	numPipelines := cfg.Workers

	// ========== Phase 2: Resolve ==========
	// Lookup all plugin factories before creating any instances (fail-fast).
	slog.Debug("resolving plugins", "task_id", cfg.ID)

	capFactory, err := plugin.GetCapturerFactory(cfg.Capture.Name)
	if err != nil {
		return fmt.Errorf("capturer %q: %w", cfg.Capture.Name, err)
	}

	parserFactories := make([]plugin.ParserFactory, len(cfg.Parsers))
	for i, pc := range cfg.Parsers {
		f, err := plugin.GetParserFactory(pc.Name)
		if err != nil {
			return fmt.Errorf("parser %q: %w", pc.Name, err)
		}
		parserFactories[i] = f
	}

	processorFactories := make([]plugin.ProcessorFactory, len(cfg.Processors))
	for i, pc := range cfg.Processors {
		f, err := plugin.GetProcessorFactory(pc.Name)
		if err != nil {
			return fmt.Errorf("processor %q: %w", pc.Name, err)
		}
		processorFactories[i] = f
	}

	repFactories := make([]plugin.ReporterFactory, len(cfg.Reporters))
	for i, rc := range cfg.Reporters {
		f, err := plugin.GetReporterFactory(rc.Name)
		if err != nil {
			return fmt.Errorf("reporter %q: %w", rc.Name, err)
		}
		repFactories[i] = f
	}

	// ========== Phase 3: Construct ==========
	// Create all empty instances. No Init or Wire yet.
	slog.Debug("constructing plugin instances", "task_id", cfg.ID)

	task := NewTask(cfg)

	// Capturers: binding mode = N instances, dispatch mode = 1 instance
	numCapturers := 1
	if cfg.Capture.DispatchMode == "binding" {
		numCapturers = numPipelines
	}
	task.Capturers = make([]plugin.Capturer, numCapturers)
	for i := range task.Capturers {
		task.Capturers[i] = capFactory()
	}

	// Reporters: M instances (one per configured reporter)
	task.Reporters = make([]plugin.Reporter, len(repFactories))
	for i := range repFactories {
		task.Reporters[i] = repFactories[i]()
	}

	// FlowRegistry: 1 per Task (shared across pipelines)
	task.Registry = NewFlowRegistry()

	// Decoder: 1 per Task (stateless, shared across pipelines)
	sharedDecoder := decoder.NewStandardDecoder(decoder.Config{
		Tunnels:      cfg.Decoder.Tunnels,
		IPReassembly: cfg.Decoder.IPReassembly,
	})

	// Parsers and Processors: N copies (one set per Pipeline)
	allParsers := make([][]plugin.Parser, numPipelines)
	allProcessors := make([][]plugin.Processor, numPipelines)
	for i := 0; i < numPipelines; i++ {
		allParsers[i] = make([]plugin.Parser, len(cfg.Parsers))
		for j := range cfg.Parsers {
			allParsers[i][j] = parserFactories[j]()
		}
		allProcessors[i] = make([]plugin.Processor, len(cfg.Processors))
		for j := range cfg.Processors {
			allProcessors[i][j] = processorFactories[j]()
		}
	}

	// ========== Phase 4: Init ==========
	// Inject plugin-specific config into all instances uniformly.
	slog.Debug("initializing all plugin instances", "task_id", cfg.ID)

	// Init Capturers
	for _, cap := range task.Capturers {
		if err := cap.Init(cfg.Capture.ToPluginConfig()); err != nil {
			return fmt.Errorf("capturer init failed: %w", err)
		}
	}

	// Init Reporters
	for i, rep := range task.Reporters {
		if err := rep.Init(cfg.Reporters[i].Config); err != nil {
			return fmt.Errorf("reporter %q init failed: %w", cfg.Reporters[i].Name, err)
		}
	}

	// Init Parsers and Processors (per-Pipeline instances)
	for i := 0; i < numPipelines; i++ {
		for j, parser := range allParsers[i] {
			if err := parser.Init(cfg.Parsers[j].Config); err != nil {
				return fmt.Errorf("pipeline %d parser %q init failed: %w", i, cfg.Parsers[j].Name, err)
			}
		}
		for j, proc := range allProcessors[i] {
			if err := proc.Init(cfg.Processors[j].Config); err != nil {
				return fmt.Errorf("pipeline %d processor %q init failed: %w", i, cfg.Processors[j].Name, err)
			}
		}
	}

	// ========== Phase 5: Wire ==========
	// Inject Task-level shared resources into plugins that need them.
	slog.Debug("wiring shared resources", "task_id", cfg.ID)

	for i := 0; i < numPipelines; i++ {
		for _, parser := range allParsers[i] {
			if fra, ok := parser.(plugin.FlowRegistryAware); ok {
				fra.SetFlowRegistry(task.Registry)
				slog.Debug("injected FlowRegistry into parser",
					"task_id", cfg.ID,
					"pipeline_id", i,
					"parser_name", parser.Name())
			}
		}
	}

	// ========== Phase 6: Assemble ==========
	// Build Pipelines from fully initialized and wired plugins.
	slog.Debug("assembling pipelines", "task_id", cfg.ID)

	for i := 0; i < numPipelines; i++ {
		p := pipeline.New(pipeline.Config{
			ID:         i,
			TaskID:     cfg.ID,
			AgentID:    m.agentID,
			Decoder:    sharedDecoder,
			Parsers:    allParsers[i],
			Processors: allProcessors[i],
		})
		task.Pipelines = append(task.Pipelines, p)
	}

	// Build ReporterWrappers (batching + fallback) for each reporter.
	// Build a name→reporter index for fallback resolution.
	reporterByName := make(map[string]plugin.Reporter, len(task.Reporters))
	for _, rep := range task.Reporters {
		reporterByName[rep.Name()] = rep
	}

	for i, rep := range task.Reporters {
		rcfg := cfg.Reporters[i]
		var fallback plugin.Reporter
		if rcfg.Fallback != "" {
			if fb, ok := reporterByName[rcfg.Fallback]; ok {
				fallback = fb
			} else {
				slog.Warn("fallback reporter not found, ignoring",
					"task_id", cfg.ID, "reporter", rcfg.Name, "fallback", rcfg.Fallback)
			}
		}

		var batchTimeout time.Duration
		if rcfg.BatchTimeout != "" {
			if parsed, err := time.ParseDuration(rcfg.BatchTimeout); err == nil {
				batchTimeout = parsed
			} else {
				slog.Warn("invalid batch_timeout, using default",
					"task_id", cfg.ID, "reporter", rcfg.Name, "value", rcfg.BatchTimeout, "error", err)
			}
		}

		w := NewReporterWrapper(WrapperConfig{
			Primary:      rep,
			Fallback:     fallback,
			TaskID:       cfg.ID,
			BatchSize:    rcfg.BatchSize,
			BatchTimeout: batchTimeout,
		})
		task.ReporterWrappers = append(task.ReporterWrappers, w)
	}

	// ========== Phase 7: Start ==========
	slog.Debug("starting task", "task_id", cfg.ID)

	if err := task.Start(); err != nil {
		task.cancel() // Release context resources on failed start
		return fmt.Errorf("task start failed: %w", err)
	}

	// Register task in manager and persist initial running state.
	m.tasks[cfg.ID] = task
	m.saveTask(task)

	slog.Info("task created successfully",
		"task_id", cfg.ID,
		"pipelines", numPipelines,
		"capturers", numCapturers,
		"reporters", len(cfg.Reporters),
		"dispatch_mode", cfg.Capture.DispatchMode,
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

	// Persist the final stopped state, then remove the on-disk record.
	m.saveTask(task)
	if err := m.store.Delete(taskID); err != nil {
		slog.Warn("failed to delete persisted task record", "task_id", taskID, "error", err)
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

	// Persist stopped state for all tasks before clearing.
	for _, t := range m.tasks {
		m.saveTask(t)
	}

	// Clear all tasks
	m.tasks = make(map[string]*Task)

	return lastErr
}

// UpdateMetricsInterval propagates a new metrics collection interval to all running tasks.
// This is called by Daemon.Reload() when the metrics.collect_interval config changes.
func (m *TaskManager) UpdateMetricsInterval(d time.Duration) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, t := range m.tasks {
		t.UpdateMetricsInterval(d)
	}

	slog.Info("metrics interval updated for all tasks", "interval", d, "task_count", len(m.tasks))
}

// saveTask persists the current state of a task to the configured store.
// It is safe to call without holding m.mu; it acquires only the task's own read lock.
func (m *TaskManager) saveTask(t *Task) {
	status := t.GetStatus()
	pt := PersistedTask{
		Version:       persistenceVersion,
		Config:        t.Config,
		State:         status.State,
		CreatedAt:     status.CreatedAt,
		FailureReason: status.FailureReason,
		RestartCount:  0, // incremented on auto-restart (future enhancement)
	}
	if !status.StartedAt.IsZero() {
		pt.StartedAt = &status.StartedAt
	}
	if !status.StoppedAt.IsZero() {
		pt.StoppedAt = &status.StoppedAt
	}
	if err := m.store.Save(pt); err != nil {
		slog.Warn("failed to persist task state", "task_id", t.Config.ID, "error", err)
	}
}

// Restore reads persisted tasks from the store and re-creates those that were
// active at the time of the last shutdown. Tasks in a terminal state are left
// as on-disk history only and do not consume an active task slot.
//
// autoRestart controls whether tasks in running/starting/stopping state are
// automatically re-created.
func (m *TaskManager) Restore(autoRestart bool) {
	persisted, err := m.store.List()
	if err != nil {
		slog.Error("task restore: failed to list persisted tasks", "error", err)
		return
	}

	for _, pt := range persisted {
		switch pt.State {
		case StateRunning, StateStarting, StateStopping:
			if !autoRestart {
				slog.Info("task restore: skipping active task (auto_restart=false)",
					"task_id", pt.Config.ID, "state", pt.State)
				continue
			}
			slog.Info("task restore: restarting previously active task",
				"task_id", pt.Config.ID, "last_state", pt.State)
			if err := m.Create(pt.Config); err != nil {
				slog.Error("task restore: failed to restart task",
					"task_id", pt.Config.ID, "error", err)
			}

		default:
			// Terminal states (stopped, failed, created) are on-disk history only;
			// they do not consume an active task slot.
			slog.Debug("task restore: skipping terminal task (history)",
				"task_id", pt.Config.ID, "state", pt.State)
		}
	}
}

// GCOldTasks removes persisted terminal-state task records that exceed the
// maxHistory limit. The oldest records (by CreatedAt) are pruned first.
func (m *TaskManager) GCOldTasks(maxHistory int) {
	persisted, err := m.store.List()
	if err != nil {
		slog.Warn("task GC: failed to list persisted tasks", "error", err)
		return
	}

	// Collect terminal tasks that are not currently active.
	m.mu.RLock()
	var terminal []PersistedTask
	for _, pt := range persisted {
		if _, active := m.tasks[pt.Config.ID]; active {
			continue
		}
		switch pt.State {
		case StateStopped, StateFailed, StateCreated:
			terminal = append(terminal, pt)
		}
	}
	m.mu.RUnlock()

	if len(terminal) <= maxHistory {
		return
	}

	// Sort oldest first and remove the excess.
	sort.Slice(terminal, func(i, j int) bool {
		return terminal[i].CreatedAt.Before(terminal[j].CreatedAt)
	})

	excess := len(terminal) - maxHistory
	for i := 0; i < excess; i++ {
		id := terminal[i].Config.ID
		if err := m.store.Delete(id); err != nil {
			slog.Warn("task GC: failed to delete old record", "task_id", id, "error", err)
		} else {
			slog.Info("task GC: removed old task record", "task_id", id)
		}
	}
}
