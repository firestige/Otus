package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDaemon_ReloadLogLevel(t *testing.T) {
	tmpDir := t.TempDir()

	configContent := `
otus:
  node:
    hostname: test-reload-001
  log:
    level: info
    format: text
  metrics:
    enabled: false
    collect_interval: 5s
  command_channel:
    enabled: false
`
	configPath := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "otus.sock")
	pidFile := filepath.Join(tmpDir, "otus.pid")

	d, err := New(configPath, socketPath, pidFile)
	if err != nil {
		t.Fatalf("new daemon: %v", err)
	}

	if err := d.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer d.Stop()

	// Verify initial log level
	if d.config.Log.Level != "info" {
		t.Fatalf("expected initial level info, got %s", d.config.Log.Level)
	}

	// Update config file to change log level
	newConfigContent := `
otus:
  node:
    hostname: test-reload-001
  log:
    level: debug
    format: text
  metrics:
    enabled: false
    collect_interval: 5s
  command_channel:
    enabled: false
`
	if err := os.WriteFile(configPath, []byte(newConfigContent), 0644); err != nil {
		t.Fatalf("write new config: %v", err)
	}

	// Reload
	if err := d.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	// Verify log level changed
	if d.config.Log.Level != "debug" {
		t.Fatalf("expected level debug after reload, got %s", d.config.Log.Level)
	}
}

func TestDaemon_ReloadPreservesRunningTasks(t *testing.T) {
	tmpDir := t.TempDir()

	configContent := `
otus:
  node:
    hostname: test-reload-002
  log:
    level: info
    format: text
  metrics:
    enabled: false
    collect_interval: 5s
  command_channel:
    enabled: false
`
	configPath := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "otus.sock")
	pidFile := filepath.Join(tmpDir, "otus.pid")

	d, err := New(configPath, socketPath, pidFile)
	if err != nil {
		t.Fatalf("new daemon: %v", err)
	}

	if err := d.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer d.Stop()

	// Get initial task count (should be 0 since no tasks created)
	initialCount := len(d.taskManager.List())

	// Reload should not affect task manager state
	if err := d.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	afterCount := len(d.taskManager.List())
	if initialCount != afterCount {
		t.Fatalf("task count changed after reload: %d → %d", initialCount, afterCount)
	}
}

func TestDaemon_ReloadMetricsInterval(t *testing.T) {
	tmpDir := t.TempDir()

	configContent := `
otus:
  node:
    hostname: test-reload-003
  log:
    level: info
    format: text
  metrics:
    enabled: false
    collect_interval: 5s
  command_channel:
    enabled: false
`
	configPath := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	socketPath := filepath.Join(tmpDir, "otus.sock")
	pidFile := filepath.Join(tmpDir, "otus.pid")

	d, err := New(configPath, socketPath, pidFile)
	if err != nil {
		t.Fatalf("new daemon: %v", err)
	}

	if err := d.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer d.Stop()

	// Update config with different metrics interval
	newConfigContent := `
otus:
  node:
    hostname: test-reload-003
  log:
    level: info
    format: text
  metrics:
    enabled: false
    collect_interval: 15s
  command_channel:
    enabled: false
`
	if err := os.WriteFile(configPath, []byte(newConfigContent), 0644); err != nil {
		t.Fatalf("write new config: %v", err)
	}

	// Reload should succeed (even without running tasks)
	if err := d.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	// Verify config was updated
	if d.config.Metrics.CollectInterval != "15s" {
		t.Fatalf("expected collect_interval 15s, got %s", d.config.Metrics.CollectInterval)
	}
}
