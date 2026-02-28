package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDaemon_StartStopIntegration(t *testing.T) {
	// Create temporary directory for test files
	tmpDir := t.TempDir()

	// Create minimal config file
	configPath := filepath.Join(tmpDir, "config.yml")
	configContent := `
node:
  hostname: test-daemon-001

control:
  socket: ` + filepath.Join(tmpDir, "capture-agent.sock") + `
  timeout: 30s

log:
  level: debug
  format: text
  output:
    type: file
    file:
      path: ` + filepath.Join(tmpDir, "capture-agent.log") + `
      max_size: 10
      max_backups: 3
      max_age: 7
      compress: false

metrics:
  enabled: true
  listen: 127.0.0.1:9091
  path: /metrics

command_channel:
  enabled: false
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "capture-agent.sock")
	pidFile := filepath.Join(tmpDir, "capture-agent.pid")

	// Create daemon instance
	d, err := New(configPath, socketPath, pidFile)
	if err != nil {
		t.Fatalf("failed to create daemon: %v", err)
	}

	// Start daemon
	if err := d.Start(); err != nil {
		t.Fatalf("failed to start daemon: %v", err)
	}

	// Verify PID file was created
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		t.Errorf("PID file was not created: %s", pidFile)
	}

	// Verify UDS socket was created
	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Errorf("UDS socket was not created: %s", socketPath)
	}

	// Run daemon in background
	runDone := make(chan error, 1)
	go func() {
		runDone <- d.Run()
	}()

	// Give daemon a moment to enter main loop
	time.Sleep(100 * time.Millisecond)

	// Trigger shutdown
	d.TriggerShutdown()

	// Wait for daemon to stop (with timeout)
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("daemon.Run() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not stop within timeout")
	}

	// Verify PID file was removed
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Errorf("PID file was not removed after shutdown: %s", pidFile)
	}

	// Verify socket was cleaned up
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Errorf("UDS socket was not removed after shutdown: %s", socketPath)
	}
}
