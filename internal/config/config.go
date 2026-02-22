// Package config handles global configuration loading using viper.
package config

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// GlobalConfig represents the top-level global static configuration.
// Maps to the `otus:` root key in YAML (see config-design.md §2).
type GlobalConfig struct {
	Node             NodeConfig             `mapstructure:"node"`
	Control          ControlConfig          `mapstructure:"control"`
	Kafka            GlobalKafkaConfig      `mapstructure:"kafka"`
	CommandChannel   CommandChannelConfig   `mapstructure:"command_channel"`
	Reporters        ReportersConfig        `mapstructure:"reporters"`
	Resources        ResourcesConfig        `mapstructure:"resources"`
	Backpressure     BackpressureConfig     `mapstructure:"backpressure"`
	Core             CoreConfig             `mapstructure:"core"`
	Metrics          MetricsConfig          `mapstructure:"metrics"`
	Log              LogConfig              `mapstructure:"log"`
	DataDir          string                 `mapstructure:"data_dir"`           // ADR-030: /var/lib/otus
	TaskPersistence  TaskPersistenceConfig  `mapstructure:"task_persistence"`   // ADR-030/031
}

// ─── Node Identity ───

// NodeConfig contains node identification settings.
type NodeConfig struct {
	IP       string            `mapstructure:"ip"`       // Empty = auto-detect (ADR-023)
	Hostname string            `mapstructure:"hostname"` // Empty = os.Hostname()
	Tags     map[string]string `mapstructure:"tags"`
}

// ─── Control Plane ───

// ControlConfig contains local control plane settings.
type ControlConfig struct {
	Socket  string `mapstructure:"socket"`
	PIDFile string `mapstructure:"pid_file"`
}

// ─── Kafka Global Default (ADR-024) ───

// GlobalKafkaConfig provides shared Kafka connection defaults.
// command_channel.kafka and reporters.kafka inherit from here when their fields are zero.
type GlobalKafkaConfig struct {
	Brokers []string   `mapstructure:"brokers"`
	SASL    SASLConfig `mapstructure:"sasl"`
	TLS     TLSConfig  `mapstructure:"tls"`
}

// SASLConfig contains SASL authentication settings.
type SASLConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Mechanism string `mapstructure:"mechanism"` // PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
}

// TLSConfig contains TLS settings.
type TLSConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	CACert             string `mapstructure:"ca_cert"`
	ClientCert         string `mapstructure:"client_cert"`
	ClientKey          string `mapstructure:"client_key"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
}

// ─── Command Channel ───

// CommandChannelConfig configures the remote command channel.
type CommandChannelConfig struct {
	Enabled    bool               `mapstructure:"enabled"`
	Type       string             `mapstructure:"type"` // "kafka"
	Kafka      CommandKafkaConfig `mapstructure:"kafka"`
	CommandTTL string             `mapstructure:"command_ttl"` // Default "5m"
}

// CommandKafkaConfig contains Kafka-specific command channel settings.
// Brokers/SASL/TLS inherit from GlobalKafkaConfig when empty/zero.
type CommandKafkaConfig struct {
	Brokers         []string   `mapstructure:"brokers"`
	Topic           string     `mapstructure:"topic"`
	ResponseTopic   string     `mapstructure:"response_topic"` // ADR-029: write responses here; empty = disabled
	GroupID         string     `mapstructure:"group_id"`
	AutoOffsetReset string     `mapstructure:"auto_offset_reset"`
	SASL            SASLConfig `mapstructure:"sasl"`
	TLS             TLSConfig  `mapstructure:"tls"`
}

// ─── Shared Reporter Connection ───

// ReportersConfig holds shared reporter connection configurations.
type ReportersConfig struct {
	Kafka KafkaReporterConnectionConfig `mapstructure:"kafka"`
}

// KafkaReporterConnectionConfig is the shared Kafka reporter connection config.
// Brokers/SASL/TLS inherit from GlobalKafkaConfig when empty/zero.
type KafkaReporterConnectionConfig struct {
	Brokers         []string   `mapstructure:"brokers"`
	Compression     string     `mapstructure:"compression"`
	MaxMessageBytes int        `mapstructure:"max_message_bytes"`
	SASL            SASLConfig `mapstructure:"sasl"`
	TLS             TLSConfig  `mapstructure:"tls"`
}

// ─── Resources & Backpressure ───

// ResourcesConfig contains global resource limits.
type ResourcesConfig struct {
	MaxWorkers int `mapstructure:"max_workers"` // 0 = auto (GOMAXPROCS)
}

// BackpressureConfig contains backpressure control settings.
type BackpressureConfig struct {
	PipelineChannel PipelineChannelConfig      `mapstructure:"pipeline_channel"`
	SendBuffer      SendBufferConfig           `mapstructure:"send_buffer"`
	Reporter        ReporterBackpressureConfig `mapstructure:"reporter"`
}

// PipelineChannelConfig configures the pipeline input channel.
type PipelineChannelConfig struct {
	Capacity   int    `mapstructure:"capacity"`
	DropPolicy string `mapstructure:"drop_policy"` // "tail" | "head"
}

// SendBufferConfig configures the send buffer between pipelines and reporters.
type SendBufferConfig struct {
	Capacity      int     `mapstructure:"capacity"`
	DropPolicy    string  `mapstructure:"drop_policy"`
	HighWatermark float64 `mapstructure:"high_watermark"`
	LowWatermark  float64 `mapstructure:"low_watermark"`
}

// ReporterBackpressureConfig configures reporter-level backpressure.
type ReporterBackpressureConfig struct {
	SendTimeout string `mapstructure:"send_timeout"`
	MaxRetries  int    `mapstructure:"max_retries"`
}

// ─── Core Decoder ───

// CoreConfig contains core engine configuration.
type CoreConfig struct {
	Decoder DecoderGlobalConfig `mapstructure:"decoder"`
}

// DecoderGlobalConfig configures the global L2-L4 decoder (not per-task).
type DecoderGlobalConfig struct {
	Tunnel       TunnelConfig       `mapstructure:"tunnel"`
	IPReassembly IPReassemblyConfig `mapstructure:"ip_reassembly"`
}

// TunnelConfig controls tunnel decapsulation.
type TunnelConfig struct {
	VXLAN  bool `mapstructure:"vxlan"`
	GRE    bool `mapstructure:"gre"`
	Geneve bool `mapstructure:"geneve"`
	IPIP   bool `mapstructure:"ipip"`
}

// IPReassemblyConfig controls IP fragment reassembly.
type IPReassemblyConfig struct {
	Timeout      string `mapstructure:"timeout"`
	MaxFragments int    `mapstructure:"max_fragments"`
}

// ─── Metrics ───

// MetricsConfig contains Prometheus metrics settings.
type MetricsConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Listen          string `mapstructure:"listen"`
	Path            string `mapstructure:"path"`
	CollectInterval string `mapstructure:"collect_interval"` // e.g. "5s", hot-reloadable
}

// ─── Log (ADR-025) ───

// LogConfig contains logging settings.
type LogConfig struct {
	Level   string           `mapstructure:"level"`  // debug / info / warn / error
	Format  string           `mapstructure:"format"` // json / text
	Outputs LogOutputsConfig `mapstructure:"outputs"`
}

// LogOutputsConfig contains structured log output destinations.
type LogOutputsConfig struct {
	File FileOutputConfig `mapstructure:"file"`
	Loki LokiOutputConfig `mapstructure:"loki"`
}

// FileOutputConfig configures file log output.
type FileOutputConfig struct {
	Enabled  bool           `mapstructure:"enabled"`
	Path     string         `mapstructure:"path"`
	Rotation RotationConfig `mapstructure:"rotation"`
}

// RotationConfig configures log file rotation (ADR-025: numeric fields).
type RotationConfig struct {
	MaxSizeMB  int  `mapstructure:"max_size_mb"`  // MB
	MaxAgeDays int  `mapstructure:"max_age_days"` // Days
	MaxBackups int  `mapstructure:"max_backups"`
	Compress   bool `mapstructure:"compress"`
}

// LokiOutputConfig configures Loki log output.
type LokiOutputConfig struct {
	Enabled      bool              `mapstructure:"enabled"`
	Endpoint     string            `mapstructure:"endpoint"`
	Labels       map[string]string `mapstructure:"labels"`
	BatchSize    int               `mapstructure:"batch_size"`
	BatchTimeout string            `mapstructure:"batch_timeout"`
}

// ─── Task Persistence (ADR-030, ADR-031) ───

// TaskPersistenceConfig controls task state persistence and history GC.
type TaskPersistenceConfig struct {
	Enabled          bool   `mapstructure:"enabled"`           // false = disable (dev/test)
	AutoRestart      bool   `mapstructure:"auto_restart"`      // true = auto-restart running tasks on startup
	GCInterval       string `mapstructure:"gc_interval"`       // default "1h"
	MaxTaskHistory   int    `mapstructure:"max_task_history"`  // 0 = disable in-process GC
}

// ─── Loading ───

// configRoot is the top-level wrapper matching the YAML structure `otus: ...`.
type configRoot struct {
	Otus GlobalConfig `mapstructure:"otus"`
}

// Load loads configuration from file.
// The YAML file uses `otus:` as root key; env vars use OTUS_ prefix (e.g., OTUS_LOG_LEVEL).
func Load(path string) (*GlobalConfig, error) {
	v := viper.New()

	// Set config file path
	v.SetConfigFile(path)

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Environment variable overrides.
	// No explicit env prefix — the `otus.` key prefix naturally maps to `OTUS_`
	// in env vars via the key replacer (e.g., key "otus.log.level" → env "OTUS_LOG_LEVEL").
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults with "otus." prefix to match the YAML structure
	setDefaults(v)

	// Unmarshal into wrapper → extract inner GlobalConfig
	var root configRoot
	if err := v.Unmarshal(&root); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	cfg := root.Otus

	// Validate and apply defaults
	if err := cfg.ValidateAndApplyDefaults(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default values for configuration.
// All keys use "otus." prefix to match the YAML root wrapper.
func setDefaults(v *viper.Viper) {
	// Control defaults
	v.SetDefault("otus.control.pid_file", "/var/run/otus.pid")
	v.SetDefault("otus.control.socket", "/var/run/otus.sock")

	// Log defaults
	v.SetDefault("otus.log.level", "info")
	v.SetDefault("otus.log.format", "json")
	v.SetDefault("otus.log.outputs.file.enabled", false)
	v.SetDefault("otus.log.outputs.file.path", "/var/log/otus/otus.log")
	v.SetDefault("otus.log.outputs.file.rotation.max_size_mb", 100)
	v.SetDefault("otus.log.outputs.file.rotation.max_age_days", 30)
	v.SetDefault("otus.log.outputs.file.rotation.max_backups", 5)
	v.SetDefault("otus.log.outputs.file.rotation.compress", true)

	// Metrics defaults
	v.SetDefault("otus.metrics.enabled", true)
	v.SetDefault("otus.metrics.listen", ":9091")
	v.SetDefault("otus.metrics.path", "/metrics")
	v.SetDefault("otus.metrics.collect_interval", "5s")

	// Command channel defaults
	v.SetDefault("otus.command_channel.enabled", false)
	v.SetDefault("otus.command_channel.type", "kafka")
	v.SetDefault("otus.command_channel.kafka.auto_offset_reset", "latest")
	v.SetDefault("otus.command_channel.command_ttl", "5m")

	// Backpressure defaults
	v.SetDefault("otus.backpressure.pipeline_channel.capacity", 65536)
	v.SetDefault("otus.backpressure.pipeline_channel.drop_policy", "tail")
	v.SetDefault("otus.backpressure.send_buffer.capacity", 16384)
	v.SetDefault("otus.backpressure.send_buffer.drop_policy", "head")
	v.SetDefault("otus.backpressure.send_buffer.high_watermark", 0.8)
	v.SetDefault("otus.backpressure.send_buffer.low_watermark", 0.3)
	v.SetDefault("otus.backpressure.reporter.send_timeout", "3s")
	v.SetDefault("otus.backpressure.reporter.max_retries", 1)

	// Core decoder defaults
	v.SetDefault("otus.core.decoder.ip_reassembly.timeout", "30s")
	v.SetDefault("otus.core.decoder.ip_reassembly.max_fragments", 10000)

	// Task persistence defaults (ADR-030, ADR-031)
	v.SetDefault("otus.data_dir", "/var/lib/otus")
	v.SetDefault("otus.task_persistence.enabled", true)
	v.SetDefault("otus.task_persistence.auto_restart", true)
	v.SetDefault("otus.task_persistence.gc_interval", "1h")
	v.SetDefault("otus.task_persistence.max_task_history", 100)

	// Reporter defaults
	v.SetDefault("otus.reporters.kafka.compression", "snappy")
	v.SetDefault("otus.reporters.kafka.max_message_bytes", 1048576)
}

// ValidateAndApplyDefaults validates configuration and applies runtime defaults.
// Implements Kafka inheritance (ADR-024) and Node IP resolution (ADR-023).
func (cfg *GlobalConfig) ValidateAndApplyDefaults() error {
	// ── Log validation ──
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[cfg.Log.Level] {
		return fmt.Errorf("invalid log level: %s (must be debug/info/warn/error)", cfg.Log.Level)
	}
	if cfg.Log.Format != "json" && cfg.Log.Format != "text" {
		return fmt.Errorf("invalid log format: %s (must be json/text)", cfg.Log.Format)
	}

	// ── Node hostname auto-detect ──
	if cfg.Node.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %w", err)
		}
		cfg.Node.Hostname = hostname
	}

	// ── Node IP resolution (ADR-023) ──
	resolvedIP, err := resolveNodeIP(&cfg.Node)
	if err != nil {
		return err
	}
	cfg.Node.IP = resolvedIP

	// ── Kafka inheritance (ADR-024) ──
	applyKafkaInheritance(cfg)

	// ── Command channel validation ──
	if cfg.CommandChannel.Enabled {
		if cfg.CommandChannel.Type != "kafka" {
			return fmt.Errorf("unsupported command_channel.type: %s (only 'kafka' supported)", cfg.CommandChannel.Type)
		}
		if len(cfg.CommandChannel.Kafka.Brokers) == 0 {
			return fmt.Errorf("command_channel.kafka.brokers is required when command_channel.enabled=true")
		}
		if cfg.CommandChannel.Kafka.Topic == "" {
			return fmt.Errorf("command_channel.kafka.topic is required when command_channel.enabled=true")
		}
		if cfg.CommandChannel.Kafka.GroupID == "" {
			cfg.CommandChannel.Kafka.GroupID = "otus-" + cfg.Node.Hostname
		}
	}

	return nil
}

// resolveNodeIP resolves the node IP address (ADR-023).
// Priority: env/config explicit value → auto-detect → error.
func resolveNodeIP(node *NodeConfig) (string, error) {
	// 1. Explicit value from config/env (Viper already merged)
	if node.IP != "" {
		return node.IP, nil
	}

	// 2. Auto-detect: first non-loopback, non-link-local IPv4
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("cannot resolve node IP: failed to list interfaces: %w", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			// Skip link-local 169.254.x.x
			if ip4[0] == 169 && ip4[1] == 254 {
				continue
			}
			return ip4.String(), nil
		}
	}

	return "", fmt.Errorf("cannot resolve node IP: set OTUS_NODE_IP or otus.node.ip")
}

// applyKafkaInheritance applies ADR-024 Kafka global config inheritance.
// Global otus.kafka fields are inherited by command_channel.kafka and reporters.kafka
// when their local fields are empty/zero.
func applyKafkaInheritance(cfg *GlobalConfig) {
	global := &cfg.Kafka

	// ── command_channel.kafka ──
	cc := &cfg.CommandChannel.Kafka
	if len(cc.Brokers) == 0 {
		cc.Brokers = global.Brokers
	}
	if !cc.SASL.Enabled && global.SASL.Enabled {
		cc.SASL = global.SASL
	}
	if !cc.TLS.Enabled && global.TLS.Enabled {
		cc.TLS = global.TLS
	}

	// ── reporters.kafka ──
	rk := &cfg.Reporters.Kafka
	if len(rk.Brokers) == 0 {
		rk.Brokers = global.Brokers
	}
	if !rk.SASL.Enabled && global.SASL.Enabled {
		rk.SASL = global.SASL
	}
	if !rk.TLS.Enabled && global.TLS.Enabled {
		rk.TLS = global.TLS
	}
}
