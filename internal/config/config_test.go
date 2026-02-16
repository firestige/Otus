package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	configContent := `
daemon:
  pid_file: "/tmp/test.pid"
  socket_path: "/tmp/test.sock"
log:
  level: "debug"
  format: "json"
  outputs:
    - type: "console"
      enabled: true
kafka:
  brokers:
    - "localhost:9092"
  command_topic: "test-commands"
  command_group: "test-group"
metrics:
  enabled: true
  listen: "0.0.0.0:9090"
  path: "/metrics"
agent:
  id: "test-agent"
  tags:
    env: "test"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate loaded values
	if cfg.Daemon.PIDFile != "/tmp/test.pid" {
		t.Errorf("Expected PIDFile /tmp/test.pid, got %s", cfg.Daemon.PIDFile)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Expected log level debug, got %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Expected log format json, got %s", cfg.Log.Format)
	}
	if len(cfg.Kafka.Brokers) != 1 || cfg.Kafka.Brokers[0] != "localhost:9092" {
		t.Errorf("Expected Kafka broker localhost:9092, got %v", cfg.Kafka.Brokers)
	}
	if cfg.Metrics.Enabled != true {
		t.Errorf("Expected metrics enabled true, got %v", cfg.Metrics.Enabled)
	}
	if cfg.Agent.ID != "test-agent" {
		t.Errorf("Expected agent ID test-agent, got %s", cfg.Agent.ID)
	}
}

func TestLoadInvalidLogLevel(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	configContent := `
log:
  level: "invalid"
  format: "json"
kafka:
  brokers:
    - "localhost:9092"
  command_topic: "test-commands"
  command_group: "test-group"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Expected error for invalid log level, got nil")
	}
}

func TestLoadInvalidLogFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	configContent := `
log:
  level: "info"
  format: "invalid"
kafka:
  brokers:
    - "localhost:9092"
  command_topic: "test-commands"
  command_group: "test-group"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Expected error for invalid log format, got nil")
	}
}

func TestLoadMissingKafkaBrokers(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	// Empty brokers is allowed (Kafka is optional)
	configContent := `
log:
  level: "info"
  format: "json"
kafka:
  brokers: []
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	// Empty brokers should NOT cause error (Kafka is optional)
	if err != nil {
		t.Errorf("Unexpected error for empty Kafka brokers: %v", err)
	}
}

func TestLoadAutoGenerateAgentID(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	configContent := `
log:
  level: "info"
  format: "json"
kafka:
  brokers:
    - "localhost:9092"
  command_topic: "test-commands"
  command_group: "test-group"
agent:
  id: ""
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Agent ID should be auto-generated from hostname
	if cfg.Agent.ID == "" {
		t.Error("Expected auto-generated agent ID, got empty string")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	configContent := `
log:
  level: "info"
  format: "json"
kafka:
  brokers:
    - "localhost:9092"
  command_topic: "test-commands"
  command_group: "test-group"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Set environment variable to override log level
	os.Setenv("OTUS_LOG_LEVEL", "debug")
	defer os.Unsetenv("OTUS_LOG_LEVEL")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Log level should be overridden by environment variable
	if cfg.Log.Level != "debug" {
		t.Errorf("Expected log level debug from env var, got %s", cfg.Log.Level)
	}
}

func TestLoadDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yml")

	// Minimal config without optional fields
	configContent := `
kafka:
  brokers:
    - "localhost:9092"
  command_topic: "test-commands"
  command_group: "test-group"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check defaults
	if cfg.Daemon.PIDFile != "/var/run/otus.pid" {
		t.Errorf("Expected default PIDFile /var/run/otus.pid, got %s", cfg.Daemon.PIDFile)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Expected default log level info, got %s", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Expected default log format json, got %s", cfg.Log.Format)
	}
	if cfg.Metrics.Enabled != true {
		t.Errorf("Expected default metrics enabled true, got %v", cfg.Metrics.Enabled)
	}
}
