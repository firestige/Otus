package kafka

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"
	"time"

	"icc.tech/capture-agent/internal/core"
)

// ─── Init Tests ───

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
			name:    "missing topic and topic_prefix",
			config:  map[string]any{"brokers": []any{"localhost:9092"}},
			wantErr: true,
		},
		{
			name: "valid with fixed topic",
			config: map[string]any{
				"brokers": []any{"localhost:9092"},
				"topic":   "test-topic",
			},
			wantErr: false,
		},
		{
			name: "valid with topic_prefix",
			config: map[string]any{
				"brokers":      []any{"localhost:9092"},
				"topic_prefix": "capture-agent",
			},
			wantErr: false,
		},
		{
			name: "topic and topic_prefix mutually exclusive",
			config: map[string]any{
				"brokers":      []any{"localhost:9092"},
				"topic":        "fixed-topic",
				"topic_prefix": "capture-agent",
			},
			wantErr: true,
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
				"serialization": "json",
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
		{
			name: "invalid serialization",
			config: map[string]any{
				"brokers":       []any{"localhost:9092"},
				"topic":         "test-topic",
				"serialization": "protobuf",
			},
			wantErr: true,
		},
		{
			name: "brokers as string slice",
			config: map[string]any{
				"brokers": []string{"broker1:9092", "broker2:9092"},
				"topic":   "test-topic",
			},
			wantErr: false,
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

// ─── Defaults Tests ───

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
	if r.config.Serialization != defaultSerialization {
		t.Errorf("Serialization = %s, want %s", r.config.Serialization, defaultSerialization)
	}
}

// ─── Topic Routing Tests (ADR-027) ───

func TestKafkaReporter_ResolveTopic_FixedTopic(t *testing.T) {
	r := &KafkaReporter{config: Config{Topic: "voip-packets"}}
	pkt := &core.OutputPacket{PayloadType: "sip"}

	got := r.resolveTopic(pkt)
	if got != "voip-packets" {
		t.Errorf("resolveTopic() = %s, want voip-packets", got)
	}
}

func TestKafkaReporter_ResolveTopic_DynamicPrefix(t *testing.T) {
	r := &KafkaReporter{config: Config{TopicPrefix: "capture-agent"}}

	tests := []struct {
		payloadType string
		wantTopic   string
	}{
		{"sip", "capture-agent-sip"},
		{"rtp", "capture-agent-rtp"},
		{"raw", "capture-agent-raw"},
		{"", "capture-agent-raw"}, // empty defaults to "raw"
	}

	for _, tt := range tests {
		pkt := &core.OutputPacket{PayloadType: tt.payloadType}
		got := r.resolveTopic(pkt)
		if got != tt.wantTopic {
			t.Errorf("resolveTopic(payloadType=%q) = %s, want %s", tt.payloadType, got, tt.wantTopic)
		}
	}
}

// ─── Headers Tests (ADR-028) ───

func TestKafkaReporter_BuildHeaders(t *testing.T) {
	r := &KafkaReporter{}

	now := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	pkt := &core.OutputPacket{
		TaskID:      "task-001",
		AgentID:     "agent-002",
		PayloadType: "sip",
		SrcIP:       netip.MustParseAddr("10.0.0.1"),
		DstIP:       netip.MustParseAddr("10.0.0.2"),
		SrcPort:     5060,
		DstPort:     5061,
		Timestamp:   now,
		Labels: map[string]string{
			"sip.method": "INVITE",
		},
	}

	headers := r.buildHeaders(pkt)

	// Build lookup map
	hdr := make(map[string]string)
	for _, h := range headers {
		hdr[h.Key] = string(h.Value)
	}

	// Verify core envelope headers
	checks := map[string]string{
		"task_id":      "task-001",
		"agent_id":     "agent-002",
		"payload_type": "sip",
		"src_ip":       "10.0.0.1",
		"dst_ip":       "10.0.0.2",
		"src_port":     "5060",
		"dst_port":     "5061",
	}
	for key, want := range checks {
		if got := hdr[key]; got != want {
			t.Errorf("header[%s] = %q, want %q", key, got, want)
		}
	}

	// Verify timestamp header exists
	if _, ok := hdr["timestamp"]; !ok {
		t.Error("missing timestamp header")
	}

	// Verify label header with "l." prefix
	if got := hdr["l.sip.method"]; got != "INVITE" {
		t.Errorf("header[l.sip.method] = %q, want INVITE", got)
	}

	// Total: 8 core + 1 label = 9
	if len(headers) != 9 {
		t.Errorf("header count = %d, want 9", len(headers))
	}
}

func TestKafkaReporter_BuildHeaders_NoLabels(t *testing.T) {
	r := &KafkaReporter{}
	pkt := &core.OutputPacket{
		TaskID:    "t1",
		AgentID:   "a1",
		SrcIP:     netip.MustParseAddr("1.2.3.4"),
		DstIP:     netip.MustParseAddr("5.6.7.8"),
		Timestamp: time.Now(),
	}

	headers := r.buildHeaders(pkt)
	// 8 core headers, 0 labels
	if len(headers) != 8 {
		t.Errorf("header count = %d, want 8", len(headers))
	}
}

// ─── Serialization Tests ───

func TestKafkaReporter_SerializeJSON(t *testing.T) {
	r := &KafkaReporter{config: Config{Serialization: "json"}}

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
		Protocol:    17,
		PayloadType: "sip",
		Labels: map[string]string{
			"sip.method":  "INVITE",
			"sip.call_id": "xyz789",
		},
		RawPayload: []byte("SIP/2.0 200 OK"),
	}

	data, err := r.serializeValue(pkt)
	if err != nil {
		t.Fatalf("serializeValue failed: %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Spot-check key fields
	if output["task_id"] != "task-123" {
		t.Errorf("task_id = %v, want task-123", output["task_id"])
	}
	if output["payload_type"] != "sip" {
		t.Errorf("payload_type = %v, want sip", output["payload_type"])
	}
	if output["pipeline_id"] != float64(7) {
		t.Errorf("pipeline_id = %v, want 7", output["pipeline_id"])
	}
	if output["raw_payload_len"] != float64(len(pkt.RawPayload)) {
		t.Errorf("raw_payload_len = %v, want %d", output["raw_payload_len"], len(pkt.RawPayload))
	}

	// Labels present
	labels, ok := output["labels"].(map[string]any)
	if !ok {
		t.Fatal("labels not found or wrong type")
	}
	if labels["sip.method"] != "INVITE" {
		t.Errorf("labels[sip.method] = %v, want INVITE", labels["sip.method"])
	}
}

func TestKafkaReporter_SerializeJSON_NoRawPayload(t *testing.T) {
	r := &KafkaReporter{config: Config{Serialization: "json"}}
	pkt := &core.OutputPacket{
		TaskID:    "t1",
		SrcIP:     netip.MustParseAddr("1.2.3.4"),
		DstIP:     netip.MustParseAddr("5.6.7.8"),
		Timestamp: time.Now(),
	}

	data, err := r.serializeValue(pkt)
	if err != nil {
		t.Fatalf("serializeValue failed: %v", err)
	}

	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// raw_payload_len should not be present when RawPayload is empty
	if _, ok := output["raw_payload_len"]; ok {
		t.Error("raw_payload_len should not be present for empty RawPayload")
	}
}

// ─── Lifecycle Tests ───

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

	if err := r.Start(ctx); err != nil {
		t.Errorf("Start() error = %v", err)
	}
	if err := r.Flush(ctx); err != nil {
		t.Errorf("Flush() error = %v", err)
	}
	if err := r.Stop(ctx); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestKafkaReporter_Lifecycle_TopicPrefix(t *testing.T) {
	r := NewKafkaReporter()

	config := map[string]any{
		"brokers":      []any{"localhost:9092"},
		"topic_prefix": "capture-agent",
	}

	err := r.Init(config)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	kr := r.(*KafkaReporter)
	if kr.config.TopicPrefix != "capture-agent" {
		t.Errorf("TopicPrefix = %s, want capture-agent", kr.config.TopicPrefix)
	}
	if kr.config.Topic != "" {
		t.Errorf("Topic = %s, want empty", kr.config.Topic)
	}

	ctx := context.Background()
	if err := r.Start(ctx); err != nil {
		t.Errorf("Start() error = %v", err)
	}
	if err := r.Stop(ctx); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestKafkaReporter_Report_NilPacket(t *testing.T) {
	r := NewKafkaReporter().(*KafkaReporter)
	config := map[string]any{
		"brokers": []any{"localhost:9092"},
		"topic":   "test-topic",
	}
	if err := r.Init(config); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	err := r.Report(context.Background(), nil)
	if err == nil {
		t.Error("Report(nil) should return error")
	}
}

// ─── Compression Tests ───

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

// ─── Serialization Config Tests ───

func TestKafkaReporter_SerializationConfig(t *testing.T) {
	tests := []struct {
		name string
		ser  string
		want string
	}{
		{"default", "", "json"},
		{"json explicit", "json", "json"},
		{"binary", "binary", "binary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewKafkaReporter().(*KafkaReporter)
			cfg := map[string]any{
				"brokers": []any{"localhost:9092"},
				"topic":   "test-topic",
			}
			if tt.ser != "" {
				cfg["serialization"] = tt.ser
			}
			if err := r.Init(cfg); err != nil {
				t.Fatalf("Init failed: %v", err)
			}
			if r.config.Serialization != tt.want {
				t.Errorf("Serialization = %s, want %s", r.config.Serialization, tt.want)
			}
		})
	}
}
