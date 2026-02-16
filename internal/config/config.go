// Package config handles global configuration loading using viper.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// GlobalConfig represents the global static configuration.
type GlobalConfig struct {
	Daemon  DaemonConfig  `mapstructure:"daemon"`
	Log     LogConfig     `mapstructure:"log"`
	Kafka   KafkaConfig   `mapstructure:"kafka"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Agent   AgentConfig   `mapstructure:"agent"`
}

// DaemonConfig contains daemon process settings.
type DaemonConfig struct {
	PIDFile    string `mapstructure:"pid_file"`
	SocketPath string `mapstructure:"socket_path"`
}

// LogConfig contains logging settings.
type LogConfig struct {
	Level   string         `mapstructure:"level"`  // debug / info / warn / error
	Format  string         `mapstructure:"format"` // json / text
	Outputs []OutputConfig `mapstructure:"outputs"`
}

// OutputConfig represents a log output destination.
type OutputConfig struct {
	Type          string            `mapstructure:"type"` // file / loki / stdout
	Path          string            `mapstructure:"path"`
	MaxSizeMB     int               `mapstructure:"max_size_mb"`
	MaxBackups    int               `mapstructure:"max_backups"`
	MaxAgeDays    int               `mapstructure:"max_age_days"`
	Compress      bool              `mapstructure:"compress"`
	Endpoint      string            `mapstructure:"endpoint"`
	Labels        map[string]string `mapstructure:"labels"`
	BatchSize     int               `mapstructure:"batch_size"`
	FlushInterval string            `mapstructure:"flush_interval"`
}

// KafkaConfig contains Kafka connection settings.
type KafkaConfig struct {
	Brokers      []string `mapstructure:"brokers"`
	CommandTopic string   `mapstructure:"command_topic"`
	CommandGroup string   `mapstructure:"command_group"`
}

// MetricsConfig contains Prometheus metrics settings.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Listen  string `mapstructure:"listen"`
	Path    string `mapstructure:"path"`
}

// AgentConfig contains agent identity settings.
type AgentConfig struct {
	ID   string            `mapstructure:"id"`
	Tags map[string]string `mapstructure:"tags"`
}

// Load loads configuration from file.
func Load(path string) (*GlobalConfig, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Set config file path
	v.SetConfigFile(path)

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Environment variable overrides
	v.SetEnvPrefix("OTUS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Unmarshal into struct
	var cfg GlobalConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate and apply defaults
	if err := validateAndApplyDefaults(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default values for configuration.
func setDefaults(v *viper.Viper) {
	// Daemon defaults
	v.SetDefault("daemon.pid_file", "/var/run/otus.pid")
	v.SetDefault("daemon.socket_path", "/var/run/otus.sock")

	// Log defaults
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.listen", ":9090")
	v.SetDefault("metrics.path", "/metrics")
}

// validateAndApplyDefaults validates configuration and applies runtime defaults.
func validateAndApplyDefaults(cfg *GlobalConfig) error {
	// Validate log level
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[cfg.Log.Level] {
		return fmt.Errorf("invalid log level: %s (must be debug/info/warn/error)", cfg.Log.Level)
	}

	// Validate log format
	if cfg.Log.Format != "json" && cfg.Log.Format != "text" {
		return fmt.Errorf("invalid log format: %s (must be json/text)", cfg.Log.Format)
	}

	// Auto-generate agent ID if empty
	if cfg.Agent.ID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname for agent ID: %w", err)
		}
		cfg.Agent.ID = hostname
	}

	// Validate Kafka brokers if configured
	if len(cfg.Kafka.Brokers) > 0 {
		if cfg.Kafka.CommandTopic == "" {
			return fmt.Errorf("kafka.command_topic is required when brokers are configured")
		}
		if cfg.Kafka.CommandGroup == "" {
			return fmt.Errorf("kafka.command_group is required when brokers are configured")
		}
	}

	return nil
}
