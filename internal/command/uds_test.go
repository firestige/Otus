package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"icc.tech/capture-agent/internal/task"
)

func TestUDSServerClient_Integration(t *testing.T) {
	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create handler
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	// Create server
	server := NewUDSServer(socketPath, handler)

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Wait a bit for server to start
	time.Sleep(100 * time.Millisecond)

	// Create client
	client := NewUDSClient(socketPath, 5*time.Second)

	// Test task.list
	t.Run("task.list", func(t *testing.T) {
		resp, err := client.TaskList(context.Background())
		if err != nil {
			t.Fatalf("TaskList failed: %v", err)
		}

		if resp.Error != nil {
			t.Errorf("unexpected error: %v", resp.Error.Message)
		}

		result, ok := resp.Result.(map[string]interface{})
		if !ok {
			t.Fatal("result is not a map")
		}

		if _, exists := result["tasks"]; !exists {
			t.Error("result missing 'tasks' field")
		}
	})

	// Test task.status
	t.Run("task.status", func(t *testing.T) {
		resp, err := client.TaskStatus(context.Background(), "")
		if err != nil {
			t.Fatalf("TaskStatus failed: %v", err)
		}

		if resp.Error != nil {
			t.Errorf("unexpected error: %v", resp.Error.Message)
		}
	})

	// Test ping
	t.Run("ping", func(t *testing.T) {
		err := client.Ping(context.Background())
		if err != nil {
			t.Errorf("Ping failed: %v", err)
		}
	})

	// Test unknown method
	t.Run("unknown_method", func(t *testing.T) {
		resp, err := client.Call(context.Background(), "unknown.method", nil)
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}

		if resp.Error == nil {
			t.Error("expected error for unknown method")
		}

		if resp.Error.Code != ErrCodeMethodNotFound {
			t.Errorf("error code = %d, want %d", resp.Error.Code, ErrCodeMethodNotFound)
		}
	})

	// Stop server
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server didn't stop in time")
	}

	// Verify socket file is removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("socket file not removed after server stop")
	}
}

func TestUDSClient_ConnectionError(t *testing.T) {
	// Try to connect to non-existent socket
	client := NewUDSClient("/tmp/non-existent-socket.sock", 1*time.Second)

	_, err := client.TaskList(context.Background())
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestUDSClient_Timeout(t *testing.T) {
	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-timeout.sock")

	// Create handler
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	// Create server
	server := NewUDSServer(socketPath, handler)

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Create client with very short timeout
	client := NewUDSClient(socketPath, 1*time.Nanosecond)

	// This should timeout
	_, err := client.TaskList(context.Background())
	if err == nil {
		t.Error("expected timeout error")
	}

	cancel()
}

func TestUDSServer_MultipleConnections(t *testing.T) {
	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-multi.sock")

	// Create handler
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	// Create server
	server := NewUDSServer(socketPath, handler)

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Create multiple clients
	clients := make([]*UDSClient, 5)
	for i := 0; i < 5; i++ {
		clients[i] = NewUDSClient(socketPath, 5*time.Second)
	}

	// Send requests concurrently
	errCh := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func(client *UDSClient) {
			_, err := client.TaskList(context.Background())
			errCh <- err
		}(clients[i])
	}

	// Wait for all responses
	for i := 0; i < 5; i++ {
		err := <-errCh
		if err != nil {
			t.Errorf("client %d failed: %v", i, err)
		}
	}

	cancel()
}

func TestUDSClient_ConvenienceMethods(t *testing.T) {
	// Create temporary socket path
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-convenience.sock")

	// Create handler
	tm := task.NewTaskManager("test-agent", nil)
	handler := NewCommandHandler(tm, nil)

	// Create server
	server := NewUDSServer(socketPath, handler)

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Create client
	client := NewUDSClient(socketPath, 5*time.Second)

	// Test convenience methods
	tests := []struct {
		name string
		fn   func() (*Response, error)
	}{
		{
			name: "TaskList",
			fn: func() (*Response, error) {
				return client.TaskList(context.Background())
			},
		},
		{
			name: "TaskStatus",
			fn: func() (*Response, error) {
				return client.TaskStatus(context.Background(), "")
			},
		},
		{
			name: "TaskDelete",
			fn: func() (*Response, error) {
				return client.TaskDelete(context.Background(), "non-existent")
			},
		},
		{
			name: "ConfigReload",
			fn: func() (*Response, error) {
				return client.ConfigReload(context.Background())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.fn()
			if err != nil {
				t.Fatalf("%s failed: %v", tt.name, err)
			}
			// Some may return errors (like TaskDelete for non-existent task)
			// but the call itself should succeed
			_ = resp
		})
	}

	cancel()
}

func TestNewUDSClient_DefaultTimeout(t *testing.T) {
	client := NewUDSClient("/tmp/test.sock", 0)
	if client.timeout != 10*time.Second {
		t.Errorf("default timeout = %v, want 10s", client.timeout)
	}

	client2 := NewUDSClient("/tmp/test.sock", 5*time.Second)
	if client2.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", client2.timeout)
	}
}
