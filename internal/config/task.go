// Package config handles configuration structures.
package config

import (
	"encoding/json"
	"fmt"
)

// TaskConfig represents dynamic per-task configuration.
type TaskConfig struct {
	ID         string            `json:"id"`
	Workers    int               `json:"workers"` // Pipeline parallelism (0 = auto)
	Capture    CaptureConfig     `json:"capture"`
	Decoder    DecoderConfig     `json:"decoder"`
	Parsers    []ParserConfig    `json:"parsers"`
	Processors []ProcessorConfig `json:"processors"`
	Reporters  []ReporterConfig  `json:"reporters"`
}

// CaptureConfig contains capture plugin configuration.
type CaptureConfig struct {
	Name         string         `json:"name"`          // af_packet_v3 / pcap / xdp
	DispatchMode string         `json:"dispatch_mode"` // "binding" | "dispatch"
	Interface    string         `json:"interface"`     // eth0 / eth1
	BPFFilter    string         `json:"bpf_filter"`    // "udp port 5060"
	SnapLen      int            `json:"snap_len"`      // Snapshot length (default 65535)
	Config       map[string]any `json:"config"`        // Plugin-specific config (fanout_group, fanout_mode, etc.)
}

// DecoderConfig contains decoder configuration.
type DecoderConfig struct {
	Tunnels      []string `json:"tunnels"`       // ["vxlan", "gre"] - tunnel types to decapsulate
	IPReassembly bool     `json:"ip_reassembly"` // Enable IP fragment reassembly
}

// ParserConfig contains parser plugin configuration.
type ParserConfig struct {
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

// ProcessorConfig contains processor plugin configuration.
type ProcessorConfig struct {
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

// ReporterConfig contains reporter plugin configuration.
type ReporterConfig struct {
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
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
