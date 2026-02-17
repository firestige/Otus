// Package command implements control plane command handling.
package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/task"
)

// CommandHandler handles control plane commands.
type CommandHandler struct {
	taskManager    *task.TaskManager
	configReloader ConfigReloader
}

// ConfigReloader is the interface for reloading global configuration.
type ConfigReloader interface {
	Reload() error
}

// NewCommandHandler creates a new command handler.
func NewCommandHandler(tm *task.TaskManager, reloader ConfigReloader) *CommandHandler {
	return &CommandHandler{
		taskManager:    tm,
		configReloader: reloader,
	}
}

// Command represents a control plane command.
type Command struct {
	Method string          `json:"method"` // e.g., "task.create", "task.delete"
	Params json.RawMessage `json:"params"` // command-specific parameters
	ID     string          `json:"id"`     // request ID for tracking
}

// Response represents a command response.
type Response struct {
	ID     string      `json:"id"`               // matches request ID
	Result interface{} `json:"result,omitempty"` // success result
	Error  *ErrorInfo  `json:"error,omitempty"`  // error info if failed
}

// ErrorInfo represents an error in the response.
type ErrorInfo struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error codes
const (
	ErrCodeParseError     = -32700 // Invalid JSON
	ErrCodeInvalidRequest = -32600 // Invalid request object
	ErrCodeMethodNotFound = -32601 // Method not found
	ErrCodeInvalidParams  = -32602 // Invalid method parameters
	ErrCodeInternalError  = -32603 // Internal error
)

// Handle processes a command and returns a response.
func (h *CommandHandler) Handle(ctx context.Context, cmd Command) Response {
	slog.Info("handling command", "method", cmd.Method, "id", cmd.ID)

	switch cmd.Method {
	case "task.create":
		return h.handleTaskCreate(ctx, cmd)
	case "task.delete":
		return h.handleTaskDelete(ctx, cmd)
	case "task.list":
		return h.handleTaskList(ctx, cmd)
	case "task.status":
		return h.handleTaskStatus(ctx, cmd)
	case "config.reload":
		return h.handleConfigReload(ctx, cmd)
	default:
		return Response{
			ID: cmd.ID,
			Error: &ErrorInfo{
				Code:    ErrCodeMethodNotFound,
				Message: fmt.Sprintf("method %q not found", cmd.Method),
			},
		}
	}
}

// TaskCreateParams represents parameters for task.create command.
type TaskCreateParams struct {
	Config config.TaskConfig `json:"config"`
}

// handleTaskCreate handles task.create command.
func (h *CommandHandler) handleTaskCreate(ctx context.Context, cmd Command) Response {
	var params TaskCreateParams
	if err := json.Unmarshal(cmd.Params, &params); err != nil {
		return Response{
			ID: cmd.ID,
			Error: &ErrorInfo{
				Code:    ErrCodeInvalidParams,
				Message: fmt.Sprintf("invalid params: %v", err),
			},
		}
	}

	err := h.taskManager.Create(params.Config)
	if err != nil {
		return Response{
			ID: cmd.ID,
			Error: &ErrorInfo{
				Code:    ErrCodeInternalError,
				Message: fmt.Sprintf("create task failed: %v", err),
			},
		}
	}

	return Response{
		ID: cmd.ID,
		Result: map[string]interface{}{
			"task_id": params.Config.ID,
			"status":  "created",
		},
	}
}

// TaskDeleteParams represents parameters for task.delete command.
type TaskDeleteParams struct {
	TaskID string `json:"task_id"`
}

// handleTaskDelete handles task.delete command.
func (h *CommandHandler) handleTaskDelete(ctx context.Context, cmd Command) Response {
	var params TaskDeleteParams
	if err := json.Unmarshal(cmd.Params, &params); err != nil {
		return Response{
			ID: cmd.ID,
			Error: &ErrorInfo{
				Code:    ErrCodeInvalidParams,
				Message: fmt.Sprintf("invalid params: %v", err),
			},
		}
	}

	err := h.taskManager.Delete(params.TaskID)
	if err != nil {
		return Response{
			ID: cmd.ID,
			Error: &ErrorInfo{
				Code:    ErrCodeInternalError,
				Message: fmt.Sprintf("delete task failed: %v", err),
			},
		}
	}

	return Response{
		ID: cmd.ID,
		Result: map[string]interface{}{
			"task_id": params.TaskID,
			"status":  "deleted",
		},
	}
}

// handleTaskList handles task.list command.
func (h *CommandHandler) handleTaskList(ctx context.Context, cmd Command) Response {
	taskIDs := h.taskManager.List()

	return Response{
		ID: cmd.ID,
		Result: map[string]interface{}{
			"tasks": taskIDs,
			"count": len(taskIDs),
		},
	}
}

// TaskStatusParams represents parameters for task.status command (optional).
type TaskStatusParams struct {
	TaskID string `json:"task_id,omitempty"` // if empty, return all
}

// handleTaskStatus handles task.status command.
func (h *CommandHandler) handleTaskStatus(ctx context.Context, cmd Command) Response {
	var params TaskStatusParams
	if len(cmd.Params) > 0 {
		if err := json.Unmarshal(cmd.Params, &params); err != nil {
			return Response{
				ID: cmd.ID,
				Error: &ErrorInfo{
					Code:    ErrCodeInvalidParams,
					Message: fmt.Sprintf("invalid params: %v", err),
				},
			}
		}
	}

	if params.TaskID != "" {
		// Get specific task status
		task, err := h.taskManager.Get(params.TaskID)
		if err != nil {
			return Response{
				ID: cmd.ID,
				Error: &ErrorInfo{
					Code:    ErrCodeInternalError,
					Message: fmt.Sprintf("get task failed: %v", err),
				},
			}
		}

		status := task.GetStatus()
		return Response{
			ID: cmd.ID,
			Result: map[string]interface{}{
				"task_id": params.TaskID,
				"status":  status.State,
			},
		}
	}

	// Get all tasks status
	statusMap := h.taskManager.Status()
	result := make(map[string]interface{})
	for id, status := range statusMap {
		result[id] = status.State
	}

	return Response{
		ID: cmd.ID,
		Result: map[string]interface{}{
			"tasks": result,
		},
	}
}

// handleConfigReload handles config.reload command.
func (h *CommandHandler) handleConfigReload(ctx context.Context, cmd Command) Response {
	if h.configReloader == nil {
		return Response{
			ID: cmd.ID,
			Error: &ErrorInfo{
				Code:    ErrCodeInternalError,
				Message: "config reloader not available",
			},
		}
	}

	err := h.configReloader.Reload()
	if err != nil {
		return Response{
			ID: cmd.ID,
			Error: &ErrorInfo{
				Code:    ErrCodeInternalError,
				Message: fmt.Sprintf("reload config failed: %v", err),
			},
		}
	}

	return Response{
		ID: cmd.ID,
		Result: map[string]interface{}{
			"status": "reloaded",
		},
	}
}
