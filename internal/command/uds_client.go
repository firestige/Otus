// Package command implements command channels.
package command

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// UDSClient is a JSON-RPC client over Unix Domain Socket.
type UDSClient struct {
	socketPath string
	timeout    time.Duration
}

// NewUDSClient creates a new UDS client.
func NewUDSClient(socketPath string, timeout time.Duration) *UDSClient {
	if timeout == 0 {
		timeout = 10 * time.Second // Default timeout
	}
	return &UDSClient{
		socketPath: socketPath,
		timeout:    timeout,
	}
}

// Call sends a command and waits for response.
func (c *UDSClient) Call(ctx context.Context, method string, params interface{}) (*Response, error) {
	// Create connection with timeout
	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to socket %s: %w", c.socketPath, err)
	}
	defer conn.Close()

	// Set deadline
	deadline := time.Now().Add(c.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	conn.SetDeadline(deadline)

	// Marshal params
	var paramsJSON json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
		paramsJSON = data
	}

	// Create JSON-RPC request
	reqID := fmt.Sprintf("req-%d", time.Now().UnixNano()) // Use string ID
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
		ID:      reqID,
	}

	// Send request
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		return nil, fmt.Errorf("connection closed without response")
	}

	// Parse JSON-RPC response
	var jsonrpcResp JSONRPCResponse
	if err := json.Unmarshal(scanner.Bytes(), &jsonrpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Verify response ID matches (convert both to string for comparison)
	respIDStr := fmt.Sprintf("%v", jsonrpcResp.ID)
	if respIDStr != reqID {
		return nil, fmt.Errorf("response ID mismatch: expected %v, got %v", reqID, respIDStr)
	}

	// Convert to internal Response format
	resp := &Response{
		ID:     fmt.Sprintf("%v", jsonrpcResp.ID),
		Result: jsonrpcResp.Result,
		Error:  jsonrpcResp.Error,
	}

	return resp, nil
}

// TaskCreate is a convenience method for task.create command.
func (c *UDSClient) TaskCreate(ctx context.Context, params TaskCreateParams) (*Response, error) {
	return c.Call(ctx, "task.create", params)
}

// TaskDelete is a convenience method for task.delete command.
func (c *UDSClient) TaskDelete(ctx context.Context, taskID string) (*Response, error) {
	return c.Call(ctx, "task.delete", TaskDeleteParams{TaskID: taskID})
}

// TaskList is a convenience method for task.list command.
func (c *UDSClient) TaskList(ctx context.Context) (*Response, error) {
	return c.Call(ctx, "task.list", nil)
}

// TaskStatus is a convenience method for task.status command.
func (c *UDSClient) TaskStatus(ctx context.Context, taskID string) (*Response, error) {
	params := TaskStatusParams{}
	if taskID != "" {
		params.TaskID = taskID
	}
	return c.Call(ctx, "task.status", params)
}

// ConfigReload is a convenience method for config.reload command.
func (c *UDSClient) ConfigReload(ctx context.Context) (*Response, error) {
	return c.Call(ctx, "config.reload", nil)
}

// Ping sends a simple ping command to check if daemon is alive.
// This is a convenience wrapper around task.list.
func (c *UDSClient) Ping(ctx context.Context) error {
	_, err := c.TaskList(ctx)
	return err
}
