package task

import (
	"testing"
	"time"
)

func TestTask_MetricsIntervalDefault(t *testing.T) {
	task := &Task{}
	interval := task.getMetricsInterval()
	if interval != 5*time.Second {
		t.Fatalf("expected default 5s, got %v", interval)
	}
}

func TestTask_UpdateMetricsInterval(t *testing.T) {
	task := &Task{}

	// Set custom interval
	task.UpdateMetricsInterval(10 * time.Second)
	if got := task.getMetricsInterval(); got != 10*time.Second {
		t.Fatalf("expected 10s, got %v", got)
	}

	// Update to different value
	task.UpdateMetricsInterval(2 * time.Second)
	if got := task.getMetricsInterval(); got != 2*time.Second {
		t.Fatalf("expected 2s, got %v", got)
	}

	// Zero duration should be ignored (keep previous value)
	task.UpdateMetricsInterval(0)
	if got := task.getMetricsInterval(); got != 2*time.Second {
		t.Fatalf("expected 2s after zero update, got %v", got)
	}

	// Negative duration should be ignored
	task.UpdateMetricsInterval(-1 * time.Second)
	if got := task.getMetricsInterval(); got != 2*time.Second {
		t.Fatalf("expected 2s after negative update, got %v", got)
	}
}

func TestTaskManager_UpdateMetricsInterval(t *testing.T) {
	m := NewTaskManager("test-agent")

	// Create mock tasks directly in the manager
	task1 := &Task{}
	task1.Config.ID = "task1"
	task2 := &Task{}
	task2.Config.ID = "task2"

	m.mu.Lock()
	m.tasks["task1"] = task1
	m.tasks["task2"] = task2
	m.mu.Unlock()

	// Update interval for all tasks
	m.UpdateMetricsInterval(15 * time.Second)

	// Verify both tasks got the update
	if got := task1.getMetricsInterval(); got != 15*time.Second {
		t.Fatalf("task1: expected 15s, got %v", got)
	}
	if got := task2.getMetricsInterval(); got != 15*time.Second {
		t.Fatalf("task2: expected 15s, got %v", got)
	}
}

func TestTaskManager_UpdateMetricsInterval_NoTasks(t *testing.T) {
	m := NewTaskManager("test-agent")

	// Should not panic with no tasks
	m.UpdateMetricsInterval(10 * time.Second)
}
