package task

import (
	"fmt"
	"sync/atomic"
	"testing"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/core/decoder"
	"firestige.xyz/otus/internal/pipeline"
	"firestige.xyz/otus/pkg/plugin"
)

// ---------------------------------------------------------------------------
// Mock implementations for lifecycle extension tests
// ---------------------------------------------------------------------------

// pausableCapturer is a mock capturer that implements Pausable.
type pausableCapturer struct {
	mockCapturer
	paused  atomic.Bool
	resumed atomic.Bool
}

func (c *pausableCapturer) Pause() error {
	c.paused.Store(true)
	return nil
}

func (c *pausableCapturer) Resume() error {
	c.resumed.Store(true)
	return nil
}

// pausableReporter is a mock reporter that implements Pausable.
type pausableReporter struct {
	mockReporter
	paused  atomic.Bool
	resumed atomic.Bool
}

func (r *pausableReporter) Pause() error {
	r.paused.Store(true)
	return nil
}

func (r *pausableReporter) Resume() error {
	r.resumed.Store(true)
	return nil
}

// reconfigurableReporter is a mock reporter that implements Reconfigurable.
type reconfigurableReporter struct {
	mockReporter
	lastConfig map[string]any
}

func (r *reconfigurableReporter) Reconfigure(cfg map[string]any) error {
	r.lastConfig = cfg
	return nil
}

// reconfigFailReporter always fails Reconfigure.
type reconfigFailReporter struct {
	mockReporter
}

func (r *reconfigFailReporter) Reconfigure(_ map[string]any) error {
	return fmt.Errorf("reconfigure refused")
}

// newLifecycleTestTask creates a running task with configurable plugins.
func newLifecycleTestTask(
	capturers []plugin.Capturer,
	reporters []plugin.Reporter,
	parsers []plugin.Parser,
	processors []plugin.Processor,
) *Task {
	cfg := config.TaskConfig{
		ID:      "test-lifecycle",
		Workers: 1,
		Capture: config.CaptureConfig{
			Name:         "mock",
			Interface:    "lo",
			DispatchMode: "binding",
			Config:       map[string]any{"interface": "lo"},
		},
		Decoder: config.DecoderConfig{},
	}
	t := NewTask(cfg)
	t.Capturers = capturers
	t.Reporters = reporters

	p := pipeline.New(pipeline.Config{
		ID:         0,
		TaskID:     cfg.ID,
		AgentID:    "test-agent",
		Decoder:    decoder.NewStandardDecoder(decoder.Config{}),
		Parsers:    parsers,
		Processors: processors,
	})
	t.Pipelines = []*pipeline.Pipeline{p}

	// Force to Running state for pause/resume tests
	t.mu.Lock()
	t.state = StateRunning
	t.mu.Unlock()

	return t
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestTask_Pause_Running(t *testing.T) {
	cap := &pausableCapturer{mockCapturer: mockCapturer{name: "cap0"}}
	rep := &pausableReporter{mockReporter: mockReporter{name: "rep0"}}

	task := newLifecycleTestTask(
		[]plugin.Capturer{cap},
		[]plugin.Reporter{rep},
		nil, nil,
	)

	if err := task.Pause(); err != nil {
		t.Fatalf("Pause() error: %v", err)
	}
	if task.State() != StatePaused {
		t.Errorf("expected StatePaused, got %s", task.State())
	}
	if !cap.paused.Load() {
		t.Error("expected capturer to be paused")
	}
	if !rep.paused.Load() {
		t.Error("expected reporter to be paused")
	}
}

func TestTask_Pause_NotRunning(t *testing.T) {
	task := newLifecycleTestTask(nil, nil, nil, nil)
	task.mu.Lock()
	task.state = StateStopped
	task.mu.Unlock()

	if err := task.Pause(); err == nil {
		t.Error("expected error pausing a stopped task")
	}
}

func TestTask_Resume_Paused(t *testing.T) {
	cap := &pausableCapturer{mockCapturer: mockCapturer{name: "cap0"}}
	rep := &pausableReporter{mockReporter: mockReporter{name: "rep0"}}

	task := newLifecycleTestTask(
		[]plugin.Capturer{cap},
		[]plugin.Reporter{rep},
		nil, nil,
	)

	// Pause first
	if err := task.Pause(); err != nil {
		t.Fatalf("Pause() error: %v", err)
	}

	// Resume
	if err := task.Resume(); err != nil {
		t.Fatalf("Resume() error: %v", err)
	}
	if task.State() != StateRunning {
		t.Errorf("expected StateRunning, got %s", task.State())
	}
	if !cap.resumed.Load() {
		t.Error("expected capturer to be resumed")
	}
	if !rep.resumed.Load() {
		t.Error("expected reporter to be resumed")
	}
}

func TestTask_Resume_NotPaused(t *testing.T) {
	task := newLifecycleTestTask(nil, nil, nil, nil)

	if err := task.Resume(); err == nil {
		t.Error("expected error resuming a running task")
	}
}

func TestTask_Pause_NonPausablePlugins(t *testing.T) {
	// Regular mock plugins don't implement Pausable — should be silently skipped.
	cap := &mockCapturer{name: "cap0"}
	rep := &mockReporter{name: "rep0"}

	task := newLifecycleTestTask(
		[]plugin.Capturer{cap},
		[]plugin.Reporter{rep},
		nil, nil,
	)

	if err := task.Pause(); err != nil {
		t.Fatalf("Pause() with non-pausable plugins should succeed, got: %v", err)
	}
	if task.State() != StatePaused {
		t.Errorf("expected StatePaused, got %s", task.State())
	}
}

func TestTask_Reconfigure_Running(t *testing.T) {
	rep := &reconfigurableReporter{mockReporter: mockReporter{name: "kafka"}}

	task := newLifecycleTestTask(
		[]plugin.Capturer{&mockCapturer{name: "cap0"}},
		[]plugin.Reporter{rep},
		nil, nil,
	)

	newCfg := map[string]map[string]any{
		"kafka": {"topic": "new-topic", "batch_size": 200},
	}

	if err := task.Reconfigure(newCfg); err != nil {
		t.Fatalf("Reconfigure() error: %v", err)
	}
	if rep.lastConfig["topic"] != "new-topic" {
		t.Errorf("expected topic 'new-topic', got %v", rep.lastConfig["topic"])
	}
}

func TestTask_Reconfigure_PluginNotFound(t *testing.T) {
	task := newLifecycleTestTask(
		[]plugin.Capturer{&mockCapturer{name: "cap0"}},
		nil, nil, nil,
	)

	err := task.Reconfigure(map[string]map[string]any{
		"nonexistent": {"key": "val"},
	})
	if err == nil {
		t.Error("expected error for nonexistent plugin")
	}
}

func TestTask_Reconfigure_NotReconfigurable(t *testing.T) {
	rep := &mockReporter{name: "plain"}

	task := newLifecycleTestTask(
		[]plugin.Capturer{&mockCapturer{name: "cap0"}},
		[]plugin.Reporter{rep},
		nil, nil,
	)

	err := task.Reconfigure(map[string]map[string]any{
		"plain": {"key": "val"},
	})
	if err == nil {
		t.Error("expected error for non-reconfigurable plugin")
	}
}

func TestTask_Reconfigure_Failure(t *testing.T) {
	rep := &reconfigFailReporter{mockReporter: mockReporter{name: "fail-rep"}}

	task := newLifecycleTestTask(
		[]plugin.Capturer{&mockCapturer{name: "cap0"}},
		[]plugin.Reporter{rep},
		nil, nil,
	)

	err := task.Reconfigure(map[string]map[string]any{
		"fail-rep": {"key": "val"},
	})
	if err == nil {
		t.Error("expected error from failing reconfigure")
	}
}

func TestTask_Reconfigure_NotRunning(t *testing.T) {
	task := newLifecycleTestTask(nil, nil, nil, nil)
	task.mu.Lock()
	task.state = StateStopped
	task.mu.Unlock()

	err := task.Reconfigure(map[string]map[string]any{
		"any": {"key": "val"},
	})
	if err == nil {
		t.Error("expected error reconfiguring a stopped task")
	}
}

// Verify Pausable and Reconfigurable interfaces are opt-in (compile-time check)
func TestLifecycleInterfaces_CompileCheck(t *testing.T) {
	var _ plugin.Pausable = (*pausableCapturer)(nil)
	var _ plugin.Pausable = (*pausableReporter)(nil)
	var _ plugin.Reconfigurable = (*reconfigurableReporter)(nil)
}
