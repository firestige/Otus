package config

import (
	"encoding/json"
	"testing"
)

func TestParseValidTaskConfig(t *testing.T) {
	configJSON := `{
		"id": "sip-capture-task-1",
		"capture": {
			"type": "afpacket",
			"interface": "eth0",
			"bpf_filter": "udp port 5060",
			"fanout_size": 4,
			"snap_len": 65535
		},
		"decoder": {
			"tunnels": ["vxlan", "gre"],
			"ip_reassembly": true
		},
		"parsers": [
			{
				"type": "sip",
				"config": {}
			}
		],
		"processors": [
			{
				"type": "filter",
				"config": {
					"field": "method",
					"value": "INVITE"
				}
			}
		],
		"reporters": [
			{
				"type": "skywalking",
				"config": {
					"endpoint": "localhost:11800"
				}
			}
		]
	}`

	tc, err := ParseTaskConfig([]byte(configJSON))
	if err != nil {
		t.Fatalf("Failed to parse task config: %v", err)
	}

	// Validate parsed values
	if tc.ID != "sip-capture-task-1" {
		t.Errorf("Expected ID sip-capture-task-1, got %s", tc.ID)
	}
	if tc.Capture.Type != "afpacket" {
		t.Errorf("Expected capture type afpacket, got %s", tc.Capture.Type)
	}
	if tc.Capture.Interface != "eth0" {
		t.Errorf("Expected interface eth0, got %s", tc.Capture.Interface)
	}
	if tc.Capture.BPFFilter != "udp port 5060" {
		t.Errorf("Expected BPF filter 'udp port 5060', got %s", tc.Capture.BPFFilter)
	}
	if tc.Capture.FanoutSize != 4 {
		t.Errorf("Expected fanout size 4, got %d", tc.Capture.FanoutSize)
	}
	if tc.Capture.SnapLen != 65535 {
		t.Errorf("Expected snap len 65535, got %d", tc.Capture.SnapLen)
	}

	// Validate decoder
	if len(tc.Decoder.Tunnels) != 2 {
		t.Errorf("Expected 2 tunnels, got %d", len(tc.Decoder.Tunnels))
	}
	if tc.Decoder.IPReassembly != true {
		t.Errorf("Expected IP reassembly true, got %v", tc.Decoder.IPReassembly)
	}

	// Validate parsers
	if len(tc.Parsers) != 1 {
		t.Fatalf("Expected 1 parser, got %d", len(tc.Parsers))
	}
	if tc.Parsers[0].Type != "sip" {
		t.Errorf("Expected parser type sip, got %s", tc.Parsers[0].Type)
	}

	// Validate processors
	if len(tc.Processors) != 1 {
		t.Errorf("Expected 1 processor, got %d", len(tc.Processors))
	}
	if tc.Processors[0].Type != "filter" {
		t.Errorf("Expected processor type filter, got %s", tc.Processors[0].Type)
	}

	// Validate reporters
	if len(tc.Reporters) != 1 {
		t.Errorf("Expected 1 reporter, got %d", len(tc.Reporters))
	}
	if tc.Reporters[0].Type != "skywalking" {
		t.Errorf("Expected reporter type skywalking, got %s", tc.Reporters[0].Type)
	}
}

func TestParseMissingTaskID(t *testing.T) {
	configJSON := `{
		"capture": {
			"type": "afpacket",
			"interface": "eth0"
		},
		"reporters": [
			{
				"type": "skywalking",
				"config": {}
			}
		]
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for missing task ID, got nil")
	}
}

func TestParseMissingCaptureType(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"interface": "eth0"
		},
		"reporters": [
			{
				"type": "skywalking",
				"config": {}
			}
		]
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for missing capture type, got nil")
	}
}

func TestParseMissingCaptureInterface(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"type": "afpacket"
		},
		"reporters": [
			{
				"type": "skywalking",
				"config": {}
			}
		]
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for missing capture interface, got nil")
	}
}

func TestParseMissingReporters(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"type": "afpacket",
			"interface": "eth0"
		},
		"reporters": []
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for missing reporters, got nil")
	}
}

func TestParseInvalidReporterType(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"type": "afpacket",
			"interface": "eth0"
		},
		"reporters": [
			{
				"type": "",
				"config": {}
			}
		]
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for invalid reporter type, got nil")
	}
}

func TestParseDefaultFanoutSize(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"type": "afpacket",
			"interface": "eth0",
			"fanout_size": 0
		},
		"reporters": [
			{
				"type": "skywalking",
				"config": {}
			}
		]
	}`

	tc, err := ParseTaskConfig([]byte(configJSON))
	if err != nil {
		t.Fatalf("Failed to parse task config: %v", err)
	}

	// FanoutSize should default to 1
	if tc.Capture.FanoutSize != 1 {
		t.Errorf("Expected default fanout size 1, got %d", tc.Capture.FanoutSize)
	}
}

func TestParseDefaultSnapLen(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"type": "afpacket",
			"interface": "eth0",
			"snap_len": 0
		},
		"reporters": [
			{
				"type": "skywalking",
				"config": {}
			}
		]
	}`

	tc, err := ParseTaskConfig([]byte(configJSON))
	if err != nil {
		t.Fatalf("Failed to parse task config: %v", err)
	}

	// SnapLen should default to 65535
	if tc.Capture.SnapLen != 65535 {
		t.Errorf("Expected default snap len 65535, got %d", tc.Capture.SnapLen)
	}
}

func TestTaskConfigMarshalUnmarshal(t *testing.T) {
	tc := &TaskConfig{
		ID: "test-task",
		Capture: CaptureConfig{
			Type:       "afpacket",
			Interface:  "eth0",
			BPFFilter:  "udp port 5060",
			FanoutSize: 4,
			SnapLen:    65535,
		},
		Decoder: DecoderConfig{
			Tunnels:      []string{"vxlan"},
			IPReassembly: true,
		},
		Parsers: []ParserConfig{
			{
				Type:   "sip",
				Config: map[string]any{},
			},
		},
		Processors: []ProcessorConfig{
			{
				Type:   "filter",
				Config: map[string]any{"field": "method"},
			},
		},
		Reporters: []ReporterConfig{
			{
				Type:   "skywalking",
				Config: map[string]any{"endpoint": "localhost:11800"},
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(tc)
	if err != nil {
		t.Fatalf("Failed to marshal task config: %v", err)
	}

	// Unmarshal back
	var tc2 TaskConfig
	if err := json.Unmarshal(data, &tc2); err != nil {
		t.Fatalf("Failed to unmarshal task config: %v", err)
	}

	// Validate
	if tc2.ID != tc.ID {
		t.Errorf("Expected ID %s, got %s", tc.ID, tc2.ID)
	}
	if tc2.Capture.Type != tc.Capture.Type {
		t.Errorf("Expected capture type %s, got %s", tc.Capture.Type, tc2.Capture.Type)
	}
}
