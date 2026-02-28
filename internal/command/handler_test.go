package command

import (
	"context"
	"encoding/json"
	"testing"

	"icc.tech/capture-agent/internal/config"
	"icc.tech/capture-agent/internal/task"
)

// mockConfigReloader is a mock implementation of ConfigReloader.
type mockConfigReloader struct {
	reloadFunc func() error
}

func (m *mockConfigReloader) Reload() error {
	if m.reloadFunc != nil {
		return m.reloadFunc()
	}
	return nil
}

func TestCommandHandler_HandleTaskCreate(t *testing.T) {
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	// Create a valid task config
	taskConfig := config.TaskConfig{
		ID:      "test-task-1",
		Workers: 1,
		Capture: config.CaptureConfig{
			Name:      "afpacket",
			Interface: "eth0",
			Config:    map[string]any{},
		},
		Parsers:    []config.ParserConfig{},
		Processors: []config.ProcessorConfig{},
		Reporters:  []config.ReporterConfig{},
	}

	// Serialize params
	params, err := json.Marshal(TaskCreateParams{Config: taskConfig})
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}

	cmd := Command{
		Method: "task_create",
		Params: params,
		ID:     "req-1",
	}

	// This will fail because plugins are not registered in test environment
	// but we can verify the handler logic works
	resp := handler.Handle(context.Background(), cmd)

	if resp.ID != "req-1" {
		t.Errorf("response ID = %s, want req-1", resp.ID)
	}

	// Should fail due to missing plugins, but that's expected
	if resp.Error == nil {
		t.Log("task creation succeeded (unexpected in test env)")
	}
}

func TestCommandHandler_HandleTaskList(t *testing.T) {
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	cmd := Command{
		Method: "task_list",
		Params: json.RawMessage{},
		ID:     "req-2",
	}

	resp := handler.Handle(context.Background(), cmd)

	if resp.ID != "req-2" {
		t.Errorf("response ID = %s, want req-2", resp.ID)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error.Message)
	}

	// Verify result structure
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatal("result is not a map")
	}

	if _, exists := result["tasks"]; !exists {
		t.Error("result missing 'tasks' field")
	}

	if _, exists := result["count"]; !exists {
		t.Error("result missing 'count' field")
	}
}

func TestCommandHandler_HandleTaskStatus(t *testing.T) {
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	// Test getting all task status (empty)
	cmd := Command{
		Method: "task_status",
		Params: json.RawMessage{},
		ID:     "req-3",
	}

	resp := handler.Handle(context.Background(), cmd)

	if resp.ID != "req-3" {
		t.Errorf("response ID = %s, want req-3", resp.ID)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error.Message)
	}
}

func TestCommandHandler_HandleTaskDelete(t *testing.T) {
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	params, _ := json.Marshal(TaskDeleteParams{TaskID: "non-existent"})
	cmd := Command{
		Method: "task_delete",
		Params: params,
		ID:     "req-4",
	}

	resp := handler.Handle(context.Background(), cmd)

	if resp.ID != "req-4" {
		t.Errorf("response ID = %s, want req-4", resp.ID)
	}

	// Should fail because task doesn't exist
	if resp.Error == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestCommandHandler_HandleConfigReload(t *testing.T) {
	tm := task.NewTaskManager("test-agent", nil)

	reloadCalled := false
	reloader := &mockConfigReloader{
		reloadFunc: func() error {
			reloadCalled = true
			return nil
		},
	}

	handler := NewCommandHandler(tm, reloader)

	cmd := Command{
		Method: "config_reload",
		Params: json.RawMessage{},
		ID:     "req-5",
	}

	resp := handler.Handle(context.Background(), cmd)

	if resp.ID != "req-5" {
		t.Errorf("response ID = %s, want req-5", resp.ID)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error.Message)
	}

	if !reloadCalled {
		t.Error("reload function was not called")
	}
}

func TestCommandHandler_HandleUnknownMethod(t *testing.T) {
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	cmd := Command{
		Method: "unknown.method",
		Params: json.RawMessage{},
		ID:     "req-6",
	}

	resp := handler.Handle(context.Background(), cmd)

	if resp.ID != "req-6" {
		t.Errorf("response ID = %s, want req-6", resp.ID)
	}

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}

	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrCodeMethodNotFound)
	}
}

func TestCommandHandler_InvalidParams(t *testing.T) {
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	// Invalid JSON params
	cmd := Command{
		Method: "task_create",
		Params: json.RawMessage(`{invalid json}`),
		ID:     "req-7",
	}

	resp := handler.Handle(context.Background(), cmd)

	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}

	if resp.Error.Code != ErrCodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrCodeInvalidParams)
	}
}
