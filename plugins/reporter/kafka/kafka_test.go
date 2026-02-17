package kafka

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"
	"time"

	"firestige.xyz/otus/internal/core"
)

func TestKafkaReporter_Init(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name:    "missing brokers",
			config:  map[string]any{"topic": "test"},
			wantErr: true,
		},
		{
			name:    "missing topic",
			config:  map[string]any{"brokers": []any{"localhost:9092"}},
			wantErr: true,
		},
		{
			name: "valid minimal config",
			config: map[string]any{
				"brokers": []any{"localhost:9092"},
				"topic":   "test-topic",
			},
			wantErr: false,
		},
		{
			name: "valid full config",
			config: map[string]any{
				"brokers":       []any{"broker1:9092", "broker2:9092"},
				"topic":         "test-topic",
				"batch_size":    float64(200),
				"batch_timeout": "200ms",
				"compression":   "gzip",
				"max_attempts":  float64(5),
			},
			wantErr: false,
		},
		{
			name: "invalid compression",
			config: map[string]any{
				"brokers":     []any{"localhost:9092"},
				"topic":       "test-topic",
				"compression": "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid batch_timeout",
			config: map[string]any{
				"brokers":       []any{"localhost:9092"},
				"topic":         "test-topic",
				"batch_timeout": "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid broker type",
			config: map[string]any{
				"brokers": []any{123},
				"topic":   "test-topic",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewKafkaReporter().(*KafkaReporter)
			err := r.Init(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestKafkaReporter_ConfigDefaults(t *testing.T) {
	r := NewKafkaReporter().(*KafkaReporter)
	config := map[string]any{
		"brokers": []any{"localhost:9092"},
		"topic":   "test-topic",
	}

	err := r.Init(config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Check defaults
	if r.config.BatchSize != defaultBatchSize {
		t.Errorf("BatchSize = %d, want %d", r.config.BatchSize, defaultBatchSize)
	}
	if r.config.BatchTimeout != defaultBatchTimeout {
		t.Errorf("BatchTimeout = %v, want %v", r.config.BatchTimeout, defaultBatchTimeout)
	}
	if r.config.Compression != defaultCompression {
		t.Errorf("Compression = %s, want %s", r.config.Compression, defaultCompression)
	}
	if r.config.MaxAttempts != defaultMaxAttempts {
		t.Errorf("MaxAttempts = %d, want %d", r.config.MaxAttempts, defaultMaxAttempts)
	}
}

func TestKafkaReporter_SerializePacket(t *testing.T) {
	r := NewKafkaReporter().(*KafkaReporter)
	config := map[string]any{
		"brokers": []any{"localhost:9092"},
		"topic":   "test-topic",
	}
	err := r.Init(config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	now := time.Now()
	pkt := &core.OutputPacket{
		TaskID:      "task-123",
		AgentID:     "agent-456",
		PipelineID:  7,
		Timestamp:   now,
		SrcIP:       netip.MustParseAddr("192.168.1.100"),
		DstIP:       netip.MustParseAddr("192.168.1.200"),
		SrcPort:     5060,
		DstPort:     5061,
		Protocol:    17, // UDP
		PayloadType: "sip",
		Labels: map[string]string{
			"sip.method":  "INVITE",
			"sip.call_id": "xyz789",
		},
		RawPayload: []byte("SIP/2.0 200 OK"),
	}

	data, err := r.serializePacket(pkt)
	if err != nil {
		t.Fatalf("serializePacket failed: %v", err)
	}

	// Parse back and verify
	var output map[string]any
	err = json.Unmarshal(data, &output)
	if err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Verify fields
	if output["task_id"] != "task-123" {
		t.Errorf("task_id = %v, want task-123", output["task_id"])
	}
	if output["agent_id"] != "agent-456" {
		t.Errorf("agent_id = %v, want agent-456", output["agent_id"])
	}
	if output["pipeline_id"] != float64(7) {
		t.Errorf("pipeline_id = %v, want 7", output["pipeline_id"])
	}
	if output["timestamp"] != float64(now.UnixMilli()) {
		t.Errorf("timestamp = %v, want %v", output["timestamp"], now.UnixMilli())
	}
	if output["src_ip"] != "192.168.1.100" {
		t.Errorf("src_ip = %v, want 192.168.1.100", output["src_ip"])
	}
	if output["dst_ip"] != "192.168.1.200" {
		t.Errorf("dst_ip = %v, want 192.168.1.200", output["dst_ip"])
	}
	if output["src_port"] != float64(5060) {
		t.Errorf("src_port = %v, want 5060", output["src_port"])
	}
	if output["dst_port"] != float64(5061) {
		t.Errorf("dst_port = %v, want 5061", output["dst_port"])
	}
	if output["protocol"] != float64(17) {
		t.Errorf("protocol = %v, want 17", output["protocol"])
	}
	if output["payload_type"] != "sip" {
		t.Errorf("payload_type = %v, want sip", output["payload_type"])
	}

	// Check labels
	labels, ok := output["labels"].(map[string]any)
	if !ok {
		t.Fatal("labels not found or wrong type")
	}
	if labels["sip.method"] != "INVITE" {
		t.Errorf("labels[sip.method] = %v, want INVITE", labels["sip.method"])
	}
	if labels["sip.call_id"] != "xyz789" {
		t.Errorf("labels[sip.call_id] = %v, want xyz789", labels["sip.call_id"])
	}

	// Check raw_payload_len
	if output["raw_payload_len"] != float64(len(pkt.RawPayload)) {
		t.Errorf("raw_payload_len = %v, want %d", output["raw_payload_len"], len(pkt.RawPayload))
	}
}

func TestKafkaReporter_Lifecycle(t *testing.T) {
	r := NewKafkaReporter()

	if name := r.Name(); name != "kafka" {
		t.Errorf("Name() = %s, want kafka", name)
	}

	config := map[string]any{
		"brokers": []any{"localhost:9092"},
		"topic":   "test-topic",
	}

	err := r.Init(config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	ctx := context.Background()

	err = r.Start(ctx)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	err = r.Flush(ctx)
	if err != nil {
		t.Errorf("Flush() error = %v", err)
	}

	err = r.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestKafkaReporter_Report_NilPacket(t *testing.T) {
	r := NewKafkaReporter().(*KafkaReporter)
	config := map[string]any{
		"brokers": []any{"localhost:9092"},
		"topic":   "test-topic",
	}
	err := r.Init(config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	err = r.Report(context.Background(), nil)
	if err == nil {
		t.Error("Report(nil) should return error")
	}
}

func TestKafkaReporter_CompressionTypes(t *testing.T) {
	compressionTypes := []string{"none", "gzip", "snappy", "lz4"}

	for _, compression := range compressionTypes {
		t.Run(compression, func(t *testing.T) {
			r := NewKafkaReporter().(*KafkaReporter)
			config := map[string]any{
				"brokers":     []any{"localhost:9092"},
				"topic":       "test-topic",
				"compression": compression,
			}
			err := r.Init(config)
			if err != nil {
				t.Errorf("Init with compression=%s failed: %v", compression, err)
			}

			if r.config.Compression != compression {
				t.Errorf("Compression = %s, want %s", r.config.Compression, compression)
			}
		})
	}
}
