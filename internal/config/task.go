// Package config handles configuration structures.
package config

import (
	"encoding/json"
	"fmt"
)

// TaskConfig represents dynamic per-task configuration.
type TaskConfig struct {
	ID         string            `json:"id"`
	Capture    CaptureConfig     `json:"capture"`
	Decoder    DecoderConfig     `json:"decoder"`
	Parsers    []string          `json:"parsers"`
	Processors []ProcessorConfig `json:"processors"`
	Reporters  []ReporterConfig  `json:"reporters"`
}

// CaptureConfig contains capture plugin configuration.
type CaptureConfig struct {
	Type       string         `json:"type"`        // afpacket / pcap / xdp
	Interface  string         `json:"interface"`   // eth0 / eth1
	BPFFilter  string         `json:"bpf_filter"`  // "udp port 5060"
	FanoutSize int            `json:"fanout_size"` // Number of parallel capture workers
	SnapLen    int            `json:"snap_len"`    // Snapshot length (default 65535)
	Extra      map[string]any `json:"extra"`       // Plugin-specific extra config
}

// DecoderConfig contains decoder configuration.
type DecoderConfig struct {
	Tunnels      []string `json:"tunnels"`       // ["vxlan", "gre"] - tunnel types to decapsulate
	IPReassembly bool     `json:"ip_reassembly"` // Enable IP fragment reassembly
}

// ProcessorConfig contains processor plugin configuration.
type ProcessorConfig struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

// ReporterConfig contains reporter plugin configuration.
type ReporterConfig struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

// Validate validates task configuration.
func (tc *TaskConfig) Validate() error {
	if tc.ID == "" {
		return fmt.Errorf("task ID is required")
	}

	// Validate capture config
	if tc.Capture.Type == "" {
		return fmt.Errorf("capture type is required")
	}
	if tc.Capture.Interface == "" {
		return fmt.Errorf("capture interface is required")
	}
	if tc.Capture.FanoutSize < 1 {
		tc.Capture.FanoutSize = 1 // Default to 1
	}
	if tc.Capture.SnapLen <= 0 {
		tc.Capture.SnapLen = 65535 // Default snap length
	}

	// At least one reporter is required
	if len(tc.Reporters) == 0 {
		return fmt.Errorf("at least one reporter is required")
	}

	// Validate reporter configs
	for i, reporter := range tc.Reporters {
		if reporter.Type == "" {
			return fmt.Errorf("reporter[%d]: type is required", i)
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
