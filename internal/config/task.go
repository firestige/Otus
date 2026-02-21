// Package config handles configuration structures.
package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TaskConfig represents dynamic per-task configuration.
type TaskConfig struct {
	ID              string                `json:"id" yaml:"id"`
	Workers         int                   `json:"workers" yaml:"workers"`
	Capture         CaptureConfig         `json:"capture" yaml:"capture"`
	Decoder         DecoderConfig         `json:"decoder" yaml:"decoder"`
	Parsers         []ParserConfig        `json:"parsers" yaml:"parsers"`
	Processors      []ProcessorConfig     `json:"processors" yaml:"processors"`
	Reporters       []ReporterConfig      `json:"reporters" yaml:"reporters"`
	ChannelCapacity ChannelCapacityConfig `json:"channel_capacity" yaml:"channel_capacity"`
}

// ChannelCapacityConfig allows tuning internal channel buffer sizes.
type ChannelCapacityConfig struct {
	RawStream  int `json:"raw_stream" yaml:"raw_stream"`   // per-pipeline input channel (default 1000)
	SendBuffer int `json:"send_buffer" yaml:"send_buffer"` // pipeline→sender channel (default 10000)
	CaptureCh  int `json:"capture_ch" yaml:"capture_ch"`   // dispatch mode intermediate channel (default 1000)
}

// CaptureConfig contains capture plugin configuration.
type CaptureConfig struct {
	Name             string         `json:"name" yaml:"name"`
	DispatchMode     string         `json:"dispatch_mode" yaml:"dispatch_mode"`
	DispatchStrategy string         `json:"dispatch_strategy" yaml:"dispatch_strategy"` // "flow-hash" (default), "round-robin"
	Interface        string         `json:"interface" yaml:"interface"`
	BPFFilter        string         `json:"bpf_filter" yaml:"bpf_filter"`
	SnapLen          int            `json:"snap_len" yaml:"snap_len"`
	Config           map[string]any `json:"config" yaml:"config"`
}

// ToPluginConfig returns the map that should be passed to plugin.Capturer.Init().
//
// Plugin Init() methods receive a map[string]any decoded from JSON, so numeric
// values that came through JSON appear as float64.  The promoted fields
// (Interface, BPFFilter, SnapLen) are first-class struct fields and are merged
// here so callers never have to repeat the promoted-field → map translation.
// Keys in Config take lower precedence and are overridden by the promoted fields.
func (c *CaptureConfig) ToPluginConfig() map[string]any {
	merged := make(map[string]any, len(c.Config)+4)
	for k, v := range c.Config {
		merged[k] = v
	}
	if c.Interface != "" {
		merged["interface"] = c.Interface
	}
	if c.BPFFilter != "" {
		merged["bpf_filter"] = c.BPFFilter
	}
	if c.SnapLen > 0 {
		// Use float64 to match how JSON numbers are stored when map[string]any
		// is populated via json.Unmarshal — keeps plugin Init() type assertions uniform.
		merged["snap_len"] = float64(c.SnapLen)
	}
	return merged
}

// DecoderConfig contains decoder configuration.
type DecoderConfig struct {
	Tunnels      []string `json:"tunnels" yaml:"tunnels"`
	IPReassembly bool     `json:"ip_reassembly" yaml:"ip_reassembly"`
}

// ParserConfig contains parser plugin configuration.
type ParserConfig struct {
	Name   string         `json:"name" yaml:"name"`
	Config map[string]any `json:"config" yaml:"config"`
}

// ProcessorConfig contains processor plugin configuration.
type ProcessorConfig struct {
	Name   string         `json:"name" yaml:"name"`
	Config map[string]any `json:"config" yaml:"config"`
}

// ReporterConfig contains reporter plugin configuration.
type ReporterConfig struct {
	Name         string         `json:"name" yaml:"name"`
	Config       map[string]any `json:"config" yaml:"config"`
	BatchSize    int            `json:"batch_size" yaml:"batch_size"`       // Wrapper batch size (default 100)
	BatchTimeout string         `json:"batch_timeout" yaml:"batch_timeout"` // Wrapper batch timeout (default 50ms)
	Fallback     string         `json:"fallback" yaml:"fallback"`           // Fallback reporter name (optional)
}

// Validate validates task configuration.
func (tc *TaskConfig) Validate() error {
	if tc.ID == "" {
		return fmt.Errorf("task ID is required")
	}

	// Validate capture config
	if tc.Capture.Name == "" {
		return fmt.Errorf("capture name is required")
	}
	if tc.Capture.Interface == "" {
		return fmt.Errorf("capture interface is required")
	}
	if tc.Capture.DispatchMode == "" {
		tc.Capture.DispatchMode = "binding" // Default to binding
	}
	if tc.Capture.DispatchMode != "binding" && tc.Capture.DispatchMode != "dispatch" {
		return fmt.Errorf("capture dispatch_mode must be 'binding' or 'dispatch', got %q", tc.Capture.DispatchMode)
	}
	if tc.Workers < 1 {
		tc.Workers = 1 // Default to 1
	}
	if tc.Capture.SnapLen <= 0 {
		tc.Capture.SnapLen = 65535 // Default snap length
	}

	// At least one reporter is required
	if len(tc.Reporters) == 0 {
		return fmt.Errorf("at least one reporter is required")
	}

	// Validate parser configs
	for i, parser := range tc.Parsers {
		if parser.Name == "" {
			return fmt.Errorf("parser[%d]: name is required", i)
		}
	}

	// Validate processor configs
	for i, processor := range tc.Processors {
		if processor.Name == "" {
			return fmt.Errorf("processor[%d]: name is required", i)
		}
	}

	// Validate reporter configs
	for i, reporter := range tc.Reporters {
		if reporter.Name == "" {
			return fmt.Errorf("reporter[%d]: name is required", i)
		}
	}

	return nil
}

// ParseTaskConfig parses task configuration from JSON.
func ParseTaskConfig(data []byte) (*TaskConfig, error) {
	var tc TaskConfig
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("failed to parse task config: %w", err)
	}

	if err := tc.Validate(); err != nil {
		return nil, err
	}

	return &tc, nil
}

// ParseTaskConfigAuto detects format (JSON/YAML) based on file extension
// and parses the task configuration accordingly.
func ParseTaskConfigAuto(data []byte, filename string) (*TaskConfig, error) {
	var tc TaskConfig

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &tc); err != nil {
			return nil, fmt.Errorf("failed to parse YAML task config: %w", err)
		}
	case ".json", "":
		if err := json.Unmarshal(data, &tc); err != nil {
			return nil, fmt.Errorf("failed to parse JSON task config: %w", err)
		}
	default:
		// Try JSON first, fall back to YAML
		if err := json.Unmarshal(data, &tc); err != nil {
			if err2 := yaml.Unmarshal(data, &tc); err2 != nil {
				return nil, fmt.Errorf("failed to parse task config (tried JSON and YAML): JSON: %v; YAML: %v", err, err2)
			}
		}
	}

	if err := tc.Validate(); err != nil {
		return nil, err
	}

	return &tc, nil
}
