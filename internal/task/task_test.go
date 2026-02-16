package task

import (
	"testing"

	"firestige.xyz/otus/internal/config"
)

func TestTaskStateTransitions(t *testing.T) {
	cfg := config.TaskConfig{
		ID: "test-task-1",
		Capture: config.CaptureConfig{
			Type:       "mock",
			Interface:  "lo",
			FanoutSize: 1,
		},
		Decoder: config.DecoderConfig{
			Tunnels:      []string{},
			IPReassembly: false,
		},
		Parsers:    []config.ParserConfig{},
		Processors: []config.ProcessorConfig{},
		Reporters: []config.ReporterConfig{
			{
				Type:   "console",
				Config: map[string]any{},
			},
		},
	}

	task := NewTask(cfg)

	// Initial state should be Created
	if task.State() != StateCreated {
		t.Errorf("Expected initial state Created, got %s", task.State())
	}

	// Test ID()
	if task.ID() != "test-task-1" {
		t.Errorf("Expected ID 'test-task-1', got %s", task.ID())
	}

	// Test GetStatus()
	status := task.GetStatus()
	if status.ID != "test-task-1" {
		t.Errorf("Expected status ID 'test-task-1', got %s", status.ID)
	}
	if status.State != StateCreated {
		t.Errorf("Expected status state Created, got %s", status.State)
	}
	if status.PipelineCount != 0 {
		t.Errorf("Expected pipeline count 0, got %d", status.PipelineCount)
	}
}

func TestTaskCreatedAttributes(t *testing.T) {
	cfg := config.TaskConfig{
		ID: "test-task-2",
		Capture: config.CaptureConfig{
			Type:       "mock",
			Interface:  "eth0",
			FanoutSize: 4,
		},
		Decoder:    config.DecoderConfig{},
		Parsers:    []config.ParserConfig{},
		Processors: []config.ProcessorConfig{},
		Reporters:  []config.ReporterConfig{},
	}

	task := NewTask(cfg)

	// Check channels are created
	if task.captureCh == nil {
		t.Error("Expected captureCh to be initialized")
	}

	if task.sendBuffer == nil {
		t.Error("Expected sendBuffer to be initialized")
	}

	if task.doneCh == nil {
		t.Error("Expected doneCh to be initialized")
	}

	// Check raw streams created based on fanout size
	expectedStreams := cfg.Capture.FanoutSize
	if expectedStreams < 1 {
		expectedStreams = 1
	}
	if len(task.rawStreams) != expectedStreams {
		t.Errorf("Expected %d raw streams, got %d", expectedStreams, len(task.rawStreams))
	}

	// Check context is created
	if task.ctx == nil {
		t.Error("Expected ctx to be initialized")
	}

	if task.cancel == nil {
		t.Error("Expected cancel func to be initialized")
	}
}

func TestTaskDefaultFanoutSize(t *testing.T) {
	cfg := config.TaskConfig{
		ID: "test-task-3",
		Capture: config.CaptureConfig{
			Type:       "mock",
			Interface:  "eth0",
			FanoutSize: 0, // Invalid, should default to 1
		},
	}

	task := NewTask(cfg)

	// Should default to 1 raw stream
	if len(task.rawStreams) != 1 {
		t.Errorf("Expected 1 raw stream for invalid fanout size, got %d", len(task.rawStreams))
	}
}

func TestTaskStateCreatedToFailed(t *testing.T) {
	cfg := config.TaskConfig{
		ID: "test-task-4",
		Capture: config.CaptureConfig{
			Type:       "nonexistent",
			Interface:  "lo",
			FanoutSize: 1,
		},
	}

	task := NewTask(cfg)

	// Manually trigger state transition to demonstrate state machine
	task.mu.Lock()
	task.setState(StateFailed)
	task.failureReason = "test failure"
	task.mu.Unlock()

	if task.State() != StateFailed {
		t.Errorf("Expected state Failed, got %s", task.State())
	}

	status := task.GetStatus()
	if status.FailureReason != "test failure" {
		t.Errorf("Expected failure reason 'test failure', got %s", status.FailureReason)
	}
}
