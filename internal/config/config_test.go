package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper to write a tmp YAML file and return its path.
func writeTmpConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write tmp config: %v", err)
	}
	return p
}

// ── Load & validate round-trip ──

func TestLoadValidConfig(t *testing.T) {
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
    hostname: "test-host"
    tags:
      env: "test"
  control:
    socket: "/tmp/test.sock"
    pid_file: "/tmp/test.pid"
  kafka:
    brokers:
      - "kafka1:9092"
  log:
    level: "debug"
    format: "json"
  metrics:
    enabled: true
    listen: "0.0.0.0:9090"
    path: "/metrics"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Node
	if cfg.Node.IP != "10.0.0.1" {
		t.Errorf("Node.IP = %q, want 10.0.0.1", cfg.Node.IP)
	}
	if cfg.Node.Hostname != "test-host" {
		t.Errorf("Node.Hostname = %q, want test-host", cfg.Node.Hostname)
	}
	if cfg.Node.Tags["env"] != "test" {
		t.Errorf("Node.Tags[env] = %q, want test", cfg.Node.Tags["env"])
	}

	// Control
	if cfg.Control.Socket != "/tmp/test.sock" {
		t.Errorf("Control.Socket = %q", cfg.Control.Socket)
	}
	if cfg.Control.PIDFile != "/tmp/test.pid" {
		t.Errorf("Control.PIDFile = %q", cfg.Control.PIDFile)
	}

	// Log
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q", cfg.Log.Format)
	}

	// Kafka global
	if len(cfg.Kafka.Brokers) != 1 || cfg.Kafka.Brokers[0] != "kafka1:9092" {
		t.Errorf("Kafka.Brokers = %v", cfg.Kafka.Brokers)
	}

	// Metrics
	if !cfg.Metrics.Enabled {
		t.Error("Metrics.Enabled = false, want true")
	}
}

// ── Log validation ──

func TestLoadInvalidLogLevel(t *testing.T) {
	_, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  log:
    level: "invalid"
    format: "json"
`))
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
	if !strings.Contains(err.Error(), "invalid log level") {
		t.Errorf("error = %v, want 'invalid log level'", err)
	}
}

func TestLoadInvalidLogFormat(t *testing.T) {
	_, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  log:
    level: "info"
    format: "invalid"
`))
	if err == nil {
		t.Fatal("expected error for invalid log format")
	}
}

// ── Node hostname auto-detect (ADR-023) ──

func TestAutoDetectHostname(t *testing.T) {
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  log:
    level: "info"
    format: "json"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Node.Hostname == "" {
		t.Error("expected auto-detected hostname, got empty")
	}
	expected, _ := os.Hostname()
	if cfg.Node.Hostname != expected {
		t.Errorf("Node.Hostname = %q, want %q", cfg.Node.Hostname, expected)
	}
}

// ── Node IP resolution (ADR-023) ──

func TestNodeIPExplicit(t *testing.T) {
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "192.168.1.100"
  log:
    level: "info"
    format: "json"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Node.IP != "192.168.1.100" {
		t.Errorf("Node.IP = %q, want 192.168.1.100", cfg.Node.IP)
	}
}

func TestNodeIPAutoDetect(t *testing.T) {
	// No explicit IP → auto-detect should find something on CI / dev containers
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  log:
    level: "info"
    format: "json"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Node.IP == "" {
		t.Error("expected auto-detected Node.IP, got empty")
	}
}

// ── Kafka inheritance (ADR-024) ──

func TestKafkaInheritanceSameCluster(t *testing.T) {
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  kafka:
    brokers:
      - "shared:9092"
    sasl:
      enabled: true
      mechanism: "PLAIN"
      username: "user"
      password: "pass"
  command_channel:
    enabled: true
    kafka:
      topic: "commands"
      group_id: "g1"
  log:
    level: "info"
    format: "json"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// command_channel should inherit global brokers and SASL
	if len(cfg.CommandChannel.Kafka.Brokers) != 1 || cfg.CommandChannel.Kafka.Brokers[0] != "shared:9092" {
		t.Errorf("CommandChannel.Kafka.Brokers = %v, want [shared:9092]", cfg.CommandChannel.Kafka.Brokers)
	}
	if cfg.CommandChannel.Kafka.SASL.Username != "user" {
		t.Errorf("CommandChannel.Kafka.SASL.Username = %q, want user", cfg.CommandChannel.Kafka.SASL.Username)
	}

	// reporters.kafka should also inherit
	if len(cfg.Reporters.Kafka.Brokers) != 1 || cfg.Reporters.Kafka.Brokers[0] != "shared:9092" {
		t.Errorf("Reporters.Kafka.Brokers = %v, want [shared:9092]", cfg.Reporters.Kafka.Brokers)
	}
	if cfg.Reporters.Kafka.SASL.Username != "user" {
		t.Errorf("Reporters.Kafka.SASL.Username = %q, want user", cfg.Reporters.Kafka.SASL.Username)
	}
}

func TestKafkaInheritanceDifferentCluster(t *testing.T) {
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  kafka:
    brokers:
      - "global:9092"
  command_channel:
    enabled: true
    kafka:
      brokers:
        - "cmd-cluster:9092"
      topic: "commands"
      group_id: "g1"
  reporters:
    kafka:
      brokers:
        - "data-cluster:9092"
  log:
    level: "info"
    format: "json"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Local overrides should take precedence
	if cfg.CommandChannel.Kafka.Brokers[0] != "cmd-cluster:9092" {
		t.Errorf("CommandChannel.Kafka.Brokers[0] = %q, want cmd-cluster:9092", cfg.CommandChannel.Kafka.Brokers[0])
	}
	if cfg.Reporters.Kafka.Brokers[0] != "data-cluster:9092" {
		t.Errorf("Reporters.Kafka.Brokers[0] = %q, want data-cluster:9092", cfg.Reporters.Kafka.Brokers[0])
	}
}

func TestKafkaInheritanceNoGlobal(t *testing.T) {
	// No global kafka, no command channel → should be fine
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  log:
    level: "info"
    format: "json"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Kafka.Brokers) != 0 {
		t.Errorf("Kafka.Brokers = %v, want empty", cfg.Kafka.Brokers)
	}
}

// ── Command channel validation ──

func TestCommandChannelEnabledWithoutBrokers(t *testing.T) {
	_, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  command_channel:
    enabled: true
    kafka:
      topic: "commands"
      group_id: "g1"
  log:
    level: "info"
    format: "json"
`))
	if err == nil {
		t.Fatal("expected error: command_channel enabled without brokers")
	}
	if !strings.Contains(err.Error(), "brokers") {
		t.Errorf("error = %v, want mention of brokers", err)
	}
}

func TestCommandChannelEnabledWithoutTopic(t *testing.T) {
	_, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  kafka:
    brokers:
      - "kafka:9092"
  command_channel:
    enabled: true
    kafka:
      group_id: "g1"
  log:
    level: "info"
    format: "json"
`))
	if err == nil {
		t.Fatal("expected error: command_channel enabled without topic")
	}
	if !strings.Contains(err.Error(), "topic") {
		t.Errorf("error = %v, want mention of topic", err)
	}
}

func TestCommandChannelAutoGroupID(t *testing.T) {
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
    hostname: "myhost"
  kafka:
    brokers:
      - "kafka:9092"
  command_channel:
    enabled: true
    kafka:
      topic: "commands"
  log:
    level: "info"
    format: "json"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.CommandChannel.Kafka.GroupID != "capture-agent-myhost" {
		t.Errorf("GroupID = %q, want capture-agent-myhost", cfg.CommandChannel.Kafka.GroupID)
	}
}

// ── Defaults ──

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Control defaults
	if cfg.Control.PIDFile != "/var/run/capture-agent.pid" {
		t.Errorf("Control.PIDFile = %q, want /var/run/capture-agent.pid", cfg.Control.PIDFile)
	}
	if cfg.Control.Socket != "/var/run/capture-agent.sock" {
		t.Errorf("Control.Socket = %q, want /var/run/capture-agent.sock", cfg.Control.Socket)
	}

	// Log defaults
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want info", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want json", cfg.Log.Format)
	}

	// Metrics defaults
	if !cfg.Metrics.Enabled {
		t.Error("Metrics.Enabled = false, want true")
	}
	if cfg.Metrics.Listen != ":9091" {
		t.Errorf("Metrics.Listen = %q, want :9091", cfg.Metrics.Listen)
	}

	// Backpressure defaults
	if cfg.Backpressure.PipelineChannel.Capacity != 65536 {
		t.Errorf("PipelineChannel.Capacity = %d, want 65536", cfg.Backpressure.PipelineChannel.Capacity)
	}
	if cfg.Backpressure.SendBuffer.HighWatermark != 0.8 {
		t.Errorf("SendBuffer.HighWatermark = %f, want 0.8", cfg.Backpressure.SendBuffer.HighWatermark)
	}

	// Reporter defaults
	if cfg.Reporters.Kafka.Compression != "snappy" {
		t.Errorf("Reporters.Kafka.Compression = %q, want snappy", cfg.Reporters.Kafka.Compression)
	}
}

// ── Env Override ──

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("CAPTURE_AGENT_LOG_LEVEL", "debug")

	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  log:
    level: "info"
    format: "json"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want debug (from env)", cfg.Log.Level)
	}
}

// ── Kafka optional (no brokers, no command channel) ──

func TestKafkaOptional(t *testing.T) {
	cfg, err := Load(writeTmpConfig(t, `
capture-agent:
  node:
    ip: "10.0.0.1"
  log:
    level: "info"
    format: "json"
`))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cfg.Kafka.Brokers) != 0 {
		t.Errorf("Kafka.Brokers = %v, want empty", cfg.Kafka.Brokers)
	}
	if cfg.CommandChannel.Enabled {
		t.Error("CommandChannel.Enabled = true, want false by default")
	}
}
