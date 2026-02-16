// Package task implements task lifecycle management.
package task

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/internal/pipeline"
	"firestige.xyz/otus/pkg/plugin"
)

// TaskState represents the state of a task in its lifecycle.
type TaskState string

const (
	// StateCreated indicates task instance created but not started.
	StateCreated TaskState = "created"
	// StateStarting indicates task is in the process of starting.
	StateStarting TaskState = "starting"
	// StateRunning indicates task is running normally.
	StateRunning TaskState = "running"
	// StateStopping indicates task is in the process of stopping.
	StateStopping TaskState = "stopping"
	// StateStopped indicates task has stopped cleanly.
	StateStopped TaskState = "stopped"
	// StateFailed indicates task failed during startup or runtime.
	StateFailed TaskState = "failed"
)

// Task represents a running packet capture task.
// It manages the complete lifecycle of a task including:
// - Capturer: 1 per Task (shared across all pipelines via FANOUT)
// - Reporter: 1 per Task (consumed by Sender goroutine)
// - Pipelines: N per Task (fanout_size from config)
// - FlowRegistry: 1 per Task (shared state across pipelines)
type Task struct {
	// Static configuration
	Config config.TaskConfig

	// Plugin instances (owned by Task)
	Capturer plugin.Capturer
	Reporter plugin.Reporter
	Registry *FlowRegistry

	// Pipeline instances (N copies)
	Pipelines []*pipeline.Pipeline

	// Runtime channels
	captureCh  chan core.RawPacket        // Capturer → Fanout
	rawStreams []chan core.RawPacket      // Fanout → Pipelines (one per pipeline)
	sendBuffer chan core.OutputPacket     // Pipelines → Sender → Reporter
	doneCh     chan struct{}              // Signals all goroutines have exited

	// State management
	mu            sync.RWMutex
	state         TaskState
	createdAt     time.Time
	startedAt     time.Time
	stoppedAt     time.Time
	failureReason string

	// Context and cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewTask creates a new task instance in Created state.
// It does NOT start the task - call Start() to begin processing.
func NewTask(cfg config.TaskConfig) *Task {
	ctx, cancel := context.WithCancel(context.Background())

	numPipelines := cfg.Capture.FanoutSize
	if numPipelines < 1 {
		numPipelines = 1
	}

	rawStreams := make([]chan core.RawPacket, numPipelines)
	for i := 0; i < numPipelines; i++ {
		rawStreams[i] = make(chan core.RawPacket, 1000) // TODO: configurable buffer size
	}

	return &Task{
		Config:     cfg,
		Pipelines:  make([]*pipeline.Pipeline, 0, numPipelines),
		captureCh:  make(chan core.RawPacket, 1000), // TODO: configurable
		rawStreams: rawStreams,
		sendBuffer: make(chan core.OutputPacket, 10000), // TODO: configurable
		doneCh:     make(chan struct{}),
		state:      StateCreated,
		createdAt:  time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// State returns the current task state.
func (t *Task) State() TaskState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

// setState updates the task state (not thread-safe, must hold mu lock).
func (t *Task) setState(s TaskState) {
	t.state = s
	slog.Info("task state changed", "task_id", t.Config.ID, "state", s)
}

// Start starts the task and transitions it to Running state.
// It starts all components in reverse dependency order:
// Reporter → Sender → Pipelines → Fanout → Capturer
// This ensures data has a destination before the source starts producing.
func (t *Task) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != StateCreated {
		return fmt.Errorf("cannot start task in state %s", t.state)
	}

	t.setState(StateStarting)
	t.startedAt = time.Now()

	// Start Reporter (data sink)
	slog.Debug("starting reporter", "task_id", t.Config.ID, "type", t.Reporter.Name())
	if err := t.Reporter.Start(t.ctx); err != nil {
		t.setState(StateFailed)
		t.failureReason = fmt.Sprintf("reporter start failed: %v", err)
		return fmt.Errorf("reporter start failed: %w", err)
	}

	// Start Sender goroutine (consumes sendBuffer → Reporter)
	go t.senderLoop()

	// Start Pipelines (processing chains)
	for i, p := range t.Pipelines {
		slog.Debug("starting pipeline", "task_id", t.Config.ID, "pipeline_id", i)
		go p.Run(t.ctx, t.rawStreams[i], t.sendBuffer)
	}

	// Start Fanout dispatcher (captureCh → rawStreams)
	go t.fanoutLoop()

	// Start Capturer (data source writes to captureCh)
	slog.Debug("starting capturer", "task_id", t.Config.ID, "type", t.Capturer.Name())
	go t.captureLoop()

	t.setState(StateRunning)
	slog.Info("task started", "task_id", t.Config.ID, "pipelines", len(t.Pipelines))

	return nil
}

// Stop stops the task gracefully.
// It stops components in forward dependency order:
// Capturer → Fanout → Pipelines → Sender → Reporter.Flush
func (t *Task) Stop() error {
	t.mu.Lock()

	if t.state != StateRunning {
		t.mu.Unlock()
		return fmt.Errorf("cannot stop task in state %s", t.state)
	}

	t.setState(StateStopping)
	t.mu.Unlock()

	slog.Info("stopping task", "task_id", t.Config.ID)

	// Step 1: Stop capturer (no more raw packets)
	slog.Debug("stopping capturer", "task_id", t.Config.ID)
	if err := t.Capturer.Stop(t.ctx); err != nil {
		slog.Warn("capturer stop error", "task_id", t.Config.ID, "error", err)
	}

	// Step 2: Close capture channel (fanout will exit)
	close(t.captureCh)

	// Step 3: Fanout will close all rawStreams when captureCh is empty
	// (Pipelines will automatically exit when rawStreams close)

	// Step 4: Cancel context to signal all goroutines
	t.cancel()

	// Step 5: Close send buffer after pipelines drain
	// Wait a bit for pipelines to finish processing
	time.Sleep(100 * time.Millisecond)
	close(t.sendBuffer)

	// Step 6: Wait for sender to finish
	<-t.doneCh

	// Step 7: Flush reporter
	slog.Debug("flushing reporter", "task_id", t.Config.ID)
	flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := t.Reporter.Flush(flushCtx); err != nil {
		slog.Warn("reporter flush error", "task_id", t.Config.ID, "error", err)
	}

	// Step 8: Stop reporter
	if err := t.Reporter.Stop(t.ctx); err != nil {
		slog.Warn("reporter stop error", "task_id", t.Config.ID, "error", err)
	}

	t.mu.Lock()
	t.setState(StateStopped)
	t.stoppedAt = time.Now()
	t.mu.Unlock()

	slog.Info("task stopped", "task_id", t.Config.ID)
	return nil
}

// captureLoop runs the capturer in a goroutine.
func (t *Task) captureLoop() {
	if err := t.Capturer.Capture(t.ctx, t.captureCh); err != nil {
		if t.ctx.Err() == nil {
			// Only log error if context wasn't cancelled
			slog.Error("capturer error", "task_id", t.Config.ID, "error", err)
			t.mu.Lock()
			t.setState(StateFailed)
			t.failureReason = fmt.Sprintf("capturer error: %v", err)
			t.mu.Unlock()
		}
	}
}

// fanoutLoop distributes packets from captureCh to all pipeline rawStreams.
// Uses round-robin distribution for load balancing.
func (t *Task) fanoutLoop() {
	defer func() {
		// Close all raw streams when fanout exits
		for i, ch := range t.rawStreams {
			close(ch)
			slog.Debug("closed raw stream", "task_id", t.Config.ID, "pipeline_id", i)
		}
	}()

	nextPipeline := 0
	numPipelines := len(t.rawStreams)

	for pkt := range t.captureCh {
		// Round-robin distribution: hash-based would be better for flow affinity
		// but round-robin is simpler and still provides load balancing
		select {
		case t.rawStreams[nextPipeline] <- pkt:
			nextPipeline = (nextPipeline + 1) % numPipelines
		case <-t.ctx.Done():
			return
		default:
			// Pipeline channel full, drop packet
			slog.Debug("pipeline channel full, dropping packet",
				"task_id", t.Config.ID,
				"pipeline_id", nextPipeline)
			nextPipeline = (nextPipeline + 1) % numPipelines
		}
	}

	slog.Debug("fanout loop exited", "task_id", t.Config.ID)
}

// senderLoop consumes OutputPackets from sendBuffer and sends them to Reporter.
// It runs until sendBuffer is closed.
func (t *Task) senderLoop() {
	defer close(t.doneCh)

	for pkt := range t.sendBuffer {
		if err := t.Reporter.Report(t.ctx, &pkt); err != nil {
			slog.Warn("reporter error", "task_id", t.Config.ID, "error", err)
			// Continue processing, don't fail the task on Reporter errors
		}
	}

	slog.Debug("sender loop exited", "task_id", t.Config.ID)
}

// Status returns a snapshot of task status.
type Status struct {
	ID            string    `json:"id"`
	State         TaskState `json:"state"`
	CreatedAt     time.Time `json:"created_at"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	StoppedAt     time.Time `json:"stopped_at,omitempty"`
	FailureReason string    `json:"failure_reason,omitempty"`
	Uptime        string    `json:"uptime,omitempty"`
	PipelineCount int       `json:"pipeline_count"`
}

// GetStatus returns current task status.
func (t *Task) GetStatus() Status {
	t.mu.RLock()
	defer t.mu.RUnlock()

	status := Status{
		ID:            t.Config.ID,
		State:         t.state,
		CreatedAt:     t.createdAt,
		StartedAt:     t.startedAt,
		StoppedAt:     t.stoppedAt,
		FailureReason: t.failureReason,
		PipelineCount: len(t.Pipelines),
	}

	if t.state == StateRunning && !t.startedAt.IsZero() {
		status.Uptime = time.Since(t.startedAt).String()
	}

	return status
}

// ID returns the task ID.
func (t *Task) ID() string {
	return t.Config.ID
}

