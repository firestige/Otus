// Package command implements control plane command handling.
package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/task"
)

// CommandHandler handles control plane commands.
type CommandHandler struct {
	taskManager    *task.TaskManager
	configReloader ConfigReloader
	shutdownFunc   func() // Called by daemon_shutdown to trigger graceful stop
	startTime      int64  // Unix timestamp of daemon start for uptime calc
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
		startTime:      time.Now().Unix(),
	}
}

// SetShutdownFunc sets the callback invoked by the daemon_shutdown command.
func (h *CommandHandler) SetShutdownFunc(fn func()) {
	h.shutdownFunc = fn
}

// Command represents a control plane command.
type Command struct {
	Method string          `json:"method"` // e.g., "task_create", "task_delete"
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
	case "task_create":
		return h.handleTaskCreate(ctx, cmd)
	case "task_delete":
		return h.handleTaskDelete(ctx, cmd)
	case "task_list":
		return h.handleTaskList(ctx, cmd)
	case "task_status":
		return h.handleTaskStatus(ctx, cmd)
	case "config_reload":
		return h.handleConfigReload(ctx, cmd)
	case "daemon_shutdown":
		return h.handleDaemonShutdown(ctx, cmd)
	case "daemon_status":
		return h.handleDaemonStatus(ctx, cmd)
	case "daemon_stats":
		return h.handleDaemonStats(ctx, cmd)
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

// TaskCreateParams represents parameters for task_create command.
type TaskCreateParams struct {
	Config config.TaskConfig `json:"config"`
}

// handleTaskCreate handles task_create command.
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

// handleDaemonShutdown triggers graceful daemon shutdown via the registered callback.
func (h *CommandHandler) handleDaemonShutdown(_ context.Context, cmd Command) Response {
	if h.shutdownFunc == nil {
		return Response{
			ID: cmd.ID,
			Error: &ErrorInfo{
				Code:    ErrCodeInternalError,
				Message: "shutdown handler not registered",
			},
		}
	}

	slog.Info("daemon_shutdown command received, initiating graceful shutdown")
	go h.shutdownFunc() // Non-blocking: let the response be sent first

	return Response{
		ID: cmd.ID,
		Result: map[string]interface{}{
			"status": "shutting_down",
		},
	}
}

// handleDaemonStatus returns daemon status information.
func (h *CommandHandler) handleDaemonStatus(_ context.Context, cmd Command) Response {
	taskIDs := h.taskManager.List()
	uptimeSeconds := time.Now().Unix() - h.startTime

	return Response{
		ID: cmd.ID,
		Result: map[string]interface{}{
			"version":    "0.1.0",
			"uptime_sec": uptimeSeconds,
			"tasks":      taskIDs,
			"task_count": len(taskIDs),
		},
	}
}

// handleDaemonStats returns runtime statistics from the task manager.
func (h *CommandHandler) handleDaemonStats(_ context.Context, cmd Command) Response {
	statusMap := h.taskManager.Status()
	taskStats := make(map[string]interface{})
	for id, status := range statusMap {
		taskStats[id] = map[string]interface{}{
			"state": status.State,
		}
	}

	return Response{
		ID: cmd.ID,
		Result: map[string]interface{}{
			"tasks": taskStats,
		},
	}
}
