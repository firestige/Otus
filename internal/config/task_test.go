package config

import (
	"encoding/json"
	"testing"
)

func TestParseValidTaskConfig(t *testing.T) {
	configJSON := `{
		"id": "sip-capture-task-1",
		"workers": 4,
		"capture": {
			"name": "afpacket",
			"dispatch_mode": "binding",
			"interface": "eth0",
			"bpf_filter": "udp port 5060",
			"snap_len": 65535
		},
		"decoder": {
			"tunnels": ["vxlan", "gre"],
			"ip_reassembly": true
		},
		"parsers": [
			{
				"name": "sip",
				"config": {}
			}
		],
		"processors": [
			{
				"name": "filter",
				"config": {
					"field": "method",
					"value": "INVITE"
				}
			}
		],
		"reporters": [
			{
				"name": "skywalking",
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
	if tc.Workers != 4 {
		t.Errorf("Expected workers 4, got %d", tc.Workers)
	}
	if tc.Capture.Name != "afpacket" {
		t.Errorf("Expected capture name afpacket, got %s", tc.Capture.Name)
	}
	if tc.Capture.DispatchMode != "binding" {
		t.Errorf("Expected dispatch mode binding, got %s", tc.Capture.DispatchMode)
	}
	if tc.Capture.Interface != "eth0" {
		t.Errorf("Expected interface eth0, got %s", tc.Capture.Interface)
	}
	if tc.Capture.BPFFilter != "udp port 5060" {
		t.Errorf("Expected BPF filter 'udp port 5060', got %s", tc.Capture.BPFFilter)
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
	if tc.Parsers[0].Name != "sip" {
		t.Errorf("Expected parser name sip, got %s", tc.Parsers[0].Name)
	}

	// Validate processors
	if len(tc.Processors) != 1 {
		t.Errorf("Expected 1 processor, got %d", len(tc.Processors))
	}
	if tc.Processors[0].Name != "filter" {
		t.Errorf("Expected processor name filter, got %s", tc.Processors[0].Name)
	}

	// Validate reporters
	if len(tc.Reporters) != 1 {
		t.Errorf("Expected 1 reporter, got %d", len(tc.Reporters))
	}
	if tc.Reporters[0].Name != "skywalking" {
		t.Errorf("Expected reporter name skywalking, got %s", tc.Reporters[0].Name)
	}
}

func TestParseMissingTaskID(t *testing.T) {
	configJSON := `{
		"capture": {
			"name": "afpacket",
			"interface": "eth0"
		},
		"reporters": [
			{
				"name": "skywalking",
				"config": {}
			}
		]
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for missing task ID, got nil")
	}
}

func TestParseMissingCaptureName(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"interface": "eth0"
		},
		"reporters": [
			{
				"name": "skywalking",
				"config": {}
			}
		]
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for missing capture name, got nil")
	}
}

func TestParseMissingCaptureInterface(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"name": "afpacket"
		},
		"reporters": [
			{
				"name": "skywalking",
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
			"name": "afpacket",
			"interface": "eth0"
		},
		"reporters": []
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for missing reporters, got nil")
	}
}

func TestParseInvalidReporterName(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"name": "afpacket",
			"interface": "eth0"
		},
		"reporters": [
			{
				"name": "",
				"config": {}
			}
		]
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for invalid reporter name, got nil")
	}
}

func TestParseInvalidDispatchMode(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"name": "afpacket",
			"interface": "eth0",
			"dispatch_mode": "invalid"
		},
		"reporters": [
			{
				"name": "skywalking",
				"config": {}
			}
		]
	}`

	_, err := ParseTaskConfig([]byte(configJSON))
	if err == nil {
		t.Error("Expected error for invalid dispatch mode, got nil")
	}
}

func TestParseDefaultWorkers(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"workers": 0,
		"capture": {
			"name": "afpacket",
			"interface": "eth0"
		},
		"reporters": [
			{
				"name": "skywalking",
				"config": {}
			}
		]
	}`

	tc, err := ParseTaskConfig([]byte(configJSON))
	if err != nil {
		t.Fatalf("Failed to parse task config: %v", err)
	}

	// Workers should default to 1
	if tc.Workers != 1 {
		t.Errorf("Expected default workers 1, got %d", tc.Workers)
	}
}

func TestParseDefaultDispatchMode(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"name": "afpacket",
			"interface": "eth0"
		},
		"reporters": [
			{
				"name": "skywalking",
				"config": {}
			}
		]
	}`

	tc, err := ParseTaskConfig([]byte(configJSON))
	if err != nil {
		t.Fatalf("Failed to parse task config: %v", err)
	}

	// DispatchMode should default to "binding"
	if tc.Capture.DispatchMode != "binding" {
		t.Errorf("Expected default dispatch mode 'binding', got %s", tc.Capture.DispatchMode)
	}
}

func TestParseDefaultSnapLen(t *testing.T) {
	configJSON := `{
		"id": "test-task",
		"capture": {
			"name": "afpacket",
			"interface": "eth0",
			"snap_len": 0
		},
		"reporters": [
			{
				"name": "skywalking",
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
		ID:      "test-task",
		Workers: 4,
		Capture: CaptureConfig{
			Name:         "afpacket",
			DispatchMode: "binding",
			Interface:    "eth0",
			BPFFilter:    "udp port 5060",
			SnapLen:      65535,
		},
		Decoder: DecoderConfig{
			Tunnels:      []string{"vxlan"},
			IPReassembly: true,
		},
		Parsers: []ParserConfig{
			{
				Name:   "sip",
				Config: map[string]any{},
			},
		},
		Processors: []ProcessorConfig{
			{
				Name:   "filter",
				Config: map[string]any{"field": "method"},
			},
		},
		Reporters: []ReporterConfig{
			{
				Name:   "skywalking",
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
	if tc2.Capture.Name != tc.Capture.Name {
		t.Errorf("Expected capture name %s, got %s", tc.Capture.Name, tc2.Capture.Name)
	}
	if tc2.Workers != tc.Workers {
		t.Errorf("Expected workers %d, got %d", tc.Workers, tc2.Workers)
	}
}
