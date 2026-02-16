package task

import (
	"testing"
)

func TestNewTaskManager(t *testing.T) {
	agentID := "test-agent-1"
	manager := NewTaskManager(agentID)

	if manager == nil {
		t.Fatal("Expected NewTaskManager to return non-nil")
	}

	if manager.agentID != agentID {
		t.Errorf("Expected agentID %s, got %s", agentID, manager.agentID)
	}

	if manager.tasks == nil {
		t.Error("Expected tasks map to be initialized")
	}
}

func TestTaskManagerList(t *testing.T) {
	manager := NewTaskManager("test-agent")

	// Empty manager
	list := manager.List()
	if len(list) != 0 {
		t.Errorf("Expected empty list, got %d items", len(list))
	}
}

func TestTaskManagerCount(t *testing.T) {
	manager := NewTaskManager("test-agent")

	// Empty manager
	if manager.Count() != 0 {
		t.Errorf("Expected count 0, got %d", manager.Count())
	}
}

func TestTaskManagerStatus(t *testing.T) {
	manager := NewTaskManager("test-agent")

	// Empty manager
	status := manager.Status()
	if len(status) != 0 {
		t.Errorf("Expected empty status map, got %d entries", len(status))
	}
}

func TestTaskManagerGet(t *testing.T) {
	manager := NewTaskManager("test-agent")

	// Get non-existent task
	_, err := manager.Get("nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent task")
	}
}

func TestTaskManagerDelete(t *testing.T) {
	manager := NewTaskManager("test-agent")

	// Delete non-existent task
	err := manager.Delete("nonexistent")
	if err == nil {
		t.Error("Expected error when deleting non-existent task")
	}
}

func TestTaskManagerStopAll(t *testing.T) {
	manager := NewTaskManager("test-agent")

	// StopAll on empty manager should not error
	err := manager.StopAll()
	if err != nil {
		t.Errorf("Expected no error from StopAll on empty manager, got %v", err)
	}

	if manager.Count() != 0 {
		t.Errorf("Expected count 0 after StopAll, got %d", manager.Count())
	}
}

// Note: Full integration tests with actual plugin registration will be in
// separate integration test files after plugins are implemented.
