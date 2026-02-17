// Package task implements task lifecycle management.
package task

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/internal/metrics"
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
// - Capturers: binding mode N / dispatch mode 1
// - Reporters: M per Task (supports horizontal scaling)
// - Pipelines: N per Task (Workers from config)
// - FlowRegistry: 1 per Task (shared state across pipelines)
type Task struct {
	// Static configuration
	Config config.TaskConfig

	// Plugin instances (owned by Task)
	Capturers        []plugin.Capturer
	Reporters        []plugin.Reporter
	ReporterWrappers []*ReporterWrapper // batching + fallback wrappers around Reporters
	Registry         *FlowRegistry

	// Pipeline instances (N copies)
	Pipelines []*pipeline.Pipeline

	// Runtime channels
	captureCh  chan core.RawPacket    // dispatch mode only: Capturer → Dispatcher
	rawStreams []chan core.RawPacket  // one per pipeline
	sendBuffer chan core.OutputPacket // Pipelines → Sender → Reporters
	doneCh     chan struct{}          // Signals sender goroutine has exited

	// Goroutine synchronization
	pipelineWg sync.WaitGroup // Tracks pipeline goroutines

	// State management
	mu            sync.RWMutex
	state         TaskState
	createdAt     time.Time
	startedAt     time.Time
	stoppedAt     time.Time
	failureReason string

	// Hot-reloadable settings
	metricsInterval atomic.Int64 // nanoseconds; 0 = use default (5s)

	// Context and cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewTask creates a new task instance in Created state.
// It does NOT start the task - call Start() to begin processing.
func NewTask(cfg config.TaskConfig) *Task {
	ctx, cancel := context.WithCancel(context.Background())

	numPipelines := cfg.Workers
	if numPipelines < 1 {
		numPipelines = 1
	}

	// Channel capacities: use configured values or sensible defaults.
	rawCap := cfg.ChannelCapacity.RawStream
	if rawCap <= 0 {
		rawCap = 1000
	}
	sendCap := cfg.ChannelCapacity.SendBuffer
	if sendCap <= 0 {
		sendCap = 10000
	}
	capCap := cfg.ChannelCapacity.CaptureCh
	if capCap <= 0 {
		capCap = 1000
	}

	rawStreams := make([]chan core.RawPacket, numPipelines)
	for i := 0; i < numPipelines; i++ {
		rawStreams[i] = make(chan core.RawPacket, rawCap)
	}

	t := &Task{
		Config:     cfg,
		Pipelines:  make([]*pipeline.Pipeline, 0, numPipelines),
		rawStreams: rawStreams,
		sendBuffer: make(chan core.OutputPacket, sendCap),
		doneCh:     make(chan struct{}),
		state:      StateCreated,
		createdAt:  time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}

	// dispatch mode needs an intermediate channel
	if cfg.Capture.DispatchMode == "dispatch" {
		t.captureCh = make(chan core.RawPacket, capCap)
	}

	return t
}

// State returns the current task state.
func (t *Task) State() TaskState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

// setState updates the task state (not thread-safe, must hold mu lock).
func (t *Task) setState(s TaskState) {
	oldState := t.state
	t.state = s
	slog.Info("task state changed", "task_id", t.Config.ID, "state", s)

	// Update Prometheus metrics
	taskID := t.Config.ID

	// Clear old state
	if oldState != "" {
		metrics.TaskStatus.WithLabelValues(taskID, string(oldState)).Set(0)
	}

	// Set new state
	var statusValue float64
	switch s {
	case StateStopped:
		statusValue = metrics.TaskStatusStopped
	case StateRunning:
		statusValue = metrics.TaskStatusRunning
	case StateFailed:
		statusValue = metrics.TaskStatusError
	default:
		// For Created, Starting, Stopping - use 0 (stopped)
		statusValue = metrics.TaskStatusStopped
	}

	metrics.TaskStatus.WithLabelValues(taskID, string(s)).Set(statusValue)
}

// Start starts the task and transitions it to Running state.
// It starts all components in reverse dependency order:
// Reporters → Sender → Pipelines → Capturers
// This ensures data has a destination before the source starts producing.
//
// If any component fails to start, already-started components are rolled back.
func (t *Task) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != StateCreated {
		return fmt.Errorf("cannot start task in state %s", t.state)
	}

	t.setState(StateStarting)
	t.startedAt = time.Now()

	// Step 1: Start Reporters (data sinks)
	startedReporters := 0
	for i, rep := range t.Reporters {
		slog.Debug("starting reporter", "task_id", t.Config.ID, "reporter_id", i, "name", rep.Name())
		if err := rep.Start(t.ctx); err != nil {
			// Rollback: stop already-started reporters
			slog.Warn("reporter start failed, rolling back", "task_id", t.Config.ID, "reporter_id", i, "error", err)
			rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 10*time.Second)
			for j := startedReporters - 1; j >= 0; j-- {
				if stopErr := t.Reporters[j].Stop(rollbackCtx); stopErr != nil {
					slog.Error("rollback: failed to stop reporter",
						"task_id", t.Config.ID, "reporter_id", j, "error", stopErr)
				}
			}
			rollbackCancel()
			t.setState(StateFailed)
			t.failureReason = fmt.Sprintf("reporter[%d] start failed: %v", i, err)
			return fmt.Errorf("reporter[%d] start failed: %w", i, err)
		}
		startedReporters++
	}

	// Step 2: Start ReporterWrappers (batching goroutines)
	for _, w := range t.ReporterWrappers {
		w.Start(t.ctx)
	}

	// Step 3: Start Sender goroutine (consumes sendBuffer → all Wrappers)
	go t.senderLoop()

	// Step 3: Start Pipelines (processing chains)
	for i, p := range t.Pipelines {
		slog.Debug("starting pipeline", "task_id", t.Config.ID, "pipeline_id", i)
		t.pipelineWg.Add(1)
		go func(idx int, pl *pipeline.Pipeline) {
			defer t.pipelineWg.Done()
			pl.Run(t.ctx, t.rawStreams[idx], t.sendBuffer)
		}(i, p)
	}

	// Step 4: Start Capturers (data sources)
	if t.Config.Capture.DispatchMode == "binding" {
		// Binding mode: each capturer writes directly to its pipeline's rawStream
		for i, cap := range t.Capturers {
			slog.Debug("starting capturer (binding)", "task_id", t.Config.ID, "capturer_id", i, "name", cap.Name())
			go t.captureLoop(cap, t.rawStreams[i])
		}
	} else {
		// Dispatch mode: single capturer → dispatcher → rawStreams
		slog.Debug("starting capturer (dispatch)", "task_id", t.Config.ID, "name", t.Capturers[0].Name())
		go t.captureLoop(t.Capturers[0], t.captureCh)
		go t.dispatchLoop()
	}

	t.setState(StateRunning)

	// Step 5: Start periodic stats collection for Prometheus metrics
	go t.statsCollectorLoop()

	slog.Info("task started", "task_id", t.Config.ID,
		"pipelines", len(t.Pipelines),
		"capturers", len(t.Capturers),
		"reporters", len(t.Reporters),
		"dispatch_mode", t.Config.Capture.DispatchMode)

	return nil
}

// Stop stops the task gracefully.
// It stops components in forward dependency order:
// Capturers → Pipelines (WaitGroup) → Sender → Reporters.Flush
func (t *Task) Stop() error {
	t.mu.Lock()

	if t.state != StateRunning {
		t.mu.Unlock()
		return fmt.Errorf("cannot stop task in state %s", t.state)
	}

	t.setState(StateStopping)
	t.mu.Unlock()

	slog.Info("stopping task", "task_id", t.Config.ID)

	// Step 1: Stop all capturers (no more raw packets)
	for i, cap := range t.Capturers {
		slog.Debug("stopping capturer", "task_id", t.Config.ID, "capturer_id", i)
		if err := cap.Stop(t.ctx); err != nil {
			slog.Warn("capturer stop error", "task_id", t.Config.ID, "capturer_id", i, "error", err)
		}
	}

	// Step 2: Close input channels so pipelines drain and exit
	if t.Config.Capture.DispatchMode == "dispatch" {
		// Close captureCh → dispatchLoop exits → closes all rawStreams
		close(t.captureCh)
	} else {
		// Binding mode: close rawStreams directly (capturers already stopped)
		for i, ch := range t.rawStreams {
			close(ch)
			slog.Debug("closed raw stream", "task_id", t.Config.ID, "pipeline_id", i)
		}
	}

	// Step 3: Wait for all pipelines to finish processing
	t.pipelineWg.Wait()

	// Step 4: Close sendBuffer (safe: pipelineWg.Wait() ensures no writers remain)
	close(t.sendBuffer)

	// Step 5: Wait for sender to finish draining sendBuffer with valid ctx
	<-t.doneCh

	// Step 6: Cancel context (senderLoop already exited, stats goroutine will exit)
	t.cancel()

	// Step 7: Flush and stop all reporters
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer flushCancel()

	for i, rep := range t.Reporters {
		slog.Debug("flushing reporter", "task_id", t.Config.ID, "reporter_id", i)
		if err := rep.Flush(flushCtx); err != nil {
			slog.Warn("reporter flush error", "task_id", t.Config.ID, "reporter_id", i, "error", err)
		}
		if err := rep.Stop(flushCtx); err != nil {
			slog.Warn("reporter stop error", "task_id", t.Config.ID, "reporter_id", i, "error", err)
		}
	}

	t.mu.Lock()
	t.setState(StateStopped)
	t.stoppedAt = time.Now()
	t.mu.Unlock()

	slog.Info("task stopped", "task_id", t.Config.ID)
	return nil
}

// captureLoop runs a single capturer, writing packets to the given output channel.
func (t *Task) captureLoop(cap plugin.Capturer, output chan<- core.RawPacket) {
	if err := cap.Capture(t.ctx, output); err != nil {
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

// dispatchLoop distributes packets from captureCh to rawStreams using flow-hash.
// Only used in dispatch mode. Guarantees flow affinity (same 5-tuple → same pipeline).
func (t *Task) dispatchLoop() {
	defer func() {
		// Close all raw streams when dispatch exits
		for i, ch := range t.rawStreams {
			close(ch)
			slog.Debug("closed raw stream", "task_id", t.Config.ID, "pipeline_id", i)
		}
	}()

	numPipelines := len(t.rawStreams)
	if numPipelines == 0 {
		slog.Error("dispatchLoop: no pipelines configured, exiting", "task_id", t.Config.ID)
		return
	}

	for pkt := range t.captureCh {
		// Flow-hash distribution for flow affinity
		idx := flowHash(pkt) % uint32(numPipelines)

		select {
		case t.rawStreams[idx] <- pkt:
		case <-t.ctx.Done():
			return
		default:
			// Pipeline channel full, drop packet
			slog.Debug("pipeline channel full, dropping packet",
				"task_id", t.Config.ID,
				"pipeline_id", idx)
		}
	}

	slog.Debug("dispatch loop exited", "task_id", t.Config.ID)
}

// flowHash computes a hash from a RawPacket's IP 5-tuple for flow-affine distribution.
// It extracts (srcIP, dstIP, srcPort, dstPort, proto) from the raw Ethernet frame.
// Falls back to hashing raw bytes if the frame cannot be parsed.
func flowHash(pkt core.RawPacket) uint32 {
	h := fnv.New32a()
	data := pkt.Data

	// Skip Ethernet header (14 bytes minimum)
	if len(data) < 14 {
		h.Write(data)
		return h.Sum32()
	}

	etherType := binary.BigEndian.Uint16(data[12:14])
	ipStart := 14

	// Handle 802.1Q VLAN tagging
	if etherType == 0x8100 {
		if len(data) < 18 {
			h.Write(data)
			return h.Sum32()
		}
		etherType = binary.BigEndian.Uint16(data[16:18])
		ipStart = 18
	}

	var proto byte

	switch etherType {
	case 0x0800: // IPv4
		ipHdr := data[ipStart:]
		if len(ipHdr) < 20 {
			h.Write(data)
			return h.Sum32()
		}
		ihl := int(ipHdr[0]&0x0F) * 4
		if ihl < 20 || len(ipHdr) < ihl {
			h.Write(data)
			return h.Sum32()
		}
		proto = ipHdr[9]
		h.Write(ipHdr[12:16]) // src IP
		h.Write(ipHdr[16:20]) // dst IP
		h.Write([]byte{proto})

		// Extract transport ports (TCP=6, UDP=17, SCTP=132)
		transHdr := ipHdr[ihl:]
		if (proto == 6 || proto == 17 || proto == 132) && len(transHdr) >= 4 {
			h.Write(transHdr[0:2]) // src port
			h.Write(transHdr[2:4]) // dst port
		}

	case 0x86DD: // IPv6
		ipHdr := data[ipStart:]
		if len(ipHdr) < 40 {
			h.Write(data)
			return h.Sum32()
		}
		proto = ipHdr[6]      // next header
		h.Write(ipHdr[8:24])  // src IP (16 bytes)
		h.Write(ipHdr[24:40]) // dst IP (16 bytes)
		h.Write([]byte{proto})

		// Extract transport ports
		transHdr := ipHdr[40:]
		if (proto == 6 || proto == 17 || proto == 132) && len(transHdr) >= 4 {
			h.Write(transHdr[0:2]) // src port
			h.Write(transHdr[2:4]) // dst port
		}

	default:
		// Non-IP frame: hash raw bytes
		n := len(data)
		if n > 64 {
			n = 64
		}
		h.Write(data[:n])
	}

	return h.Sum32()
}

// senderLoop consumes OutputPackets from sendBuffer and distributes them to ReporterWrappers.
// If no wrappers are configured, falls back to direct Reporter.Report() calls.
// It runs until sendBuffer is closed.
func (t *Task) senderLoop() {
	defer close(t.doneCh)

	if len(t.ReporterWrappers) > 0 {
		// Batched path: distribute to wrappers
		for pkt := range t.sendBuffer {
			p := pkt // copy for pointer safety
			for _, w := range t.ReporterWrappers {
				w.Send(&p)
			}
		}
		// sendBuffer closed — close all wrapper channels and wait for flush
		for _, w := range t.ReporterWrappers {
			w.Close()
		}
	} else {
		// Legacy path: direct Reporter.Report() calls (no wrappers)
		for pkt := range t.sendBuffer {
			for i, rep := range t.Reporters {
				if err := rep.Report(t.ctx, &pkt); err != nil {
					slog.Warn("reporter error", "task_id", t.Config.ID, "reporter_id", i, "error", err)
				}
			}
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

// getMetricsInterval returns the current metrics collection interval.
// If no custom interval is set (atomic value 0), defaults to 5 seconds.
func (t *Task) getMetricsInterval() time.Duration {
	ns := t.metricsInterval.Load()
	if ns <= 0 {
		return 5 * time.Second
	}
	return time.Duration(ns)
}

// UpdateMetricsInterval sets a new metrics collection interval.
// The change takes effect on the next tick of the statsCollectorLoop.
func (t *Task) UpdateMetricsInterval(d time.Duration) {
	if d > 0 {
		t.metricsInterval.Store(int64(d))
	}
}

// statsCollectorLoop periodically collects stats from capturers and updates Prometheus metrics.
// Uses per-capturer tracking to correctly compute deltas in binding mode (multiple capturers).
func (t *Task) statsCollectorLoop() {
	interval := t.getMetricsInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Per-capturer last-seen counters to avoid cross-capturer delta contamination.
	type capStats struct {
		packetsReceived uint64
		packetsDropped  uint64
	}
	lastStats := make([]capStats, len(t.Capturers))

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			// Check if interval was updated (hot-reload)
			if newInterval := t.getMetricsInterval(); newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
				slog.Info("metrics collect interval updated", "task_id", t.Config.ID, "interval", interval)
			}
			for i, cap := range t.Capturers {
				stats := cap.Stats()

				// Calculate per-capturer deltas with underflow protection
				deltaReceived := stats.PacketsReceived - lastStats[i].packetsReceived
				if stats.PacketsReceived < lastStats[i].packetsReceived {
					// Counter reset (capturer restart) — treat current value as delta
					deltaReceived = stats.PacketsReceived
				}

				deltaDropped := stats.PacketsDropped - lastStats[i].packetsDropped
				if stats.PacketsDropped < lastStats[i].packetsDropped {
					deltaDropped = stats.PacketsDropped
				}

				if deltaReceived > 0 {
					ifaceName, _ := t.Config.Capture.Config["interface"].(string)
					metrics.CapturePacketsTotal.WithLabelValues(
						t.Config.ID,
						ifaceName,
					).Add(float64(deltaReceived))
				}

				if deltaDropped > 0 {
					metrics.CaptureDropsTotal.WithLabelValues(
						t.Config.ID,
						"capture",
					).Add(float64(deltaDropped))
				}

				// Update per-capturer tracking
				lastStats[i] = capStats{
					packetsReceived: stats.PacketsReceived,
					packetsDropped:  stats.PacketsDropped,
				}

				slog.Debug("capturer stats collected",
					"task_id", t.Config.ID,
					"capturer_id", i,
					"packets_received", stats.PacketsReceived,
					"packets_dropped", stats.PacketsDropped,
					"delta_received", deltaReceived,
					"delta_dropped", deltaDropped)
			}
		}
	}
}
