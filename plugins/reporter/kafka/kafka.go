// Package kafka implements Kafka reporter plugin.
// Sends OutputPackets to Kafka with dynamic topic routing (ADR-027),
// envelope-as-headers separation (ADR-028), and configurable serialization.
package kafka

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"

	"icc.tech/capture-agent/internal/core"
	"icc.tech/capture-agent/pkg/plugin"
)

const (
	defaultBatchSize        = 100
	defaultBatchTimeout     = 100 * time.Millisecond
	defaultCompression      = "snappy"
	defaultMaxAttempts      = 3
	defaultSerialization    = "json"
	defaultProtocolFallback = "raw"
)

// KafkaReporter sends packets to Kafka.
type KafkaReporter struct {
	name   string
	writer *kafka.Writer
	config Config

	// Statistics
	reportedCount atomic.Uint64
	errorCount    atomic.Uint64
}

// Config represents Kafka reporter configuration.
type Config struct {
	// Connection — may come from capture-agent.reporters.kafka (ADR-028) or per-reporter config.
	Brokers     []string `json:"brokers"`
	Compression string   `json:"compression"`  // none|gzip|snappy|lz4, default snappy
	MaxAttempts int      `json:"max_attempts"` // default 3

	// Topic routing (ADR-027): topic and topic_prefix are mutually exclusive.
	// When topic_prefix is set, actual topic = "{prefix}-{protocol}" (e.g. "capture-agent-sip").
	Topic       string `json:"topic"`        // Fixed topic
	TopicPrefix string `json:"topic_prefix"` // Dynamic routing prefix

	// Batching
	BatchSize    int           `json:"batch_size"`    // default 100
	BatchTimeout time.Duration `json:"batch_timeout"` // default 100ms

	// Serialization format for message Value.
	// "json" = JSON envelope (Phase 1 default)
	// "binary" = future binary format via Payload interface (Phase 2)
	Serialization string `json:"serialization"` // default "json"
}

// NewKafkaReporter creates a new Kafka reporter.
func NewKafkaReporter() plugin.Reporter {
	return &KafkaReporter{
		name: "kafka",
	}
}

// Name returns the plugin name.
func (r *KafkaReporter) Name() string {
	return r.name
}

// Init initializes the reporter with configuration.
func (r *KafkaReporter) Init(config map[string]any) error {
	if config == nil {
		return fmt.Errorf("kafka reporter requires configuration")
	}

	// Apply defaults
	cfg := Config{
		BatchSize:     defaultBatchSize,
		BatchTimeout:  defaultBatchTimeout,
		Compression:   defaultCompression,
		MaxAttempts:   defaultMaxAttempts,
		Serialization: defaultSerialization,
	}

	// Required: brokers
	if brokers, ok := config["brokers"].([]any); ok {
		cfg.Brokers = make([]string, len(brokers))
		for i, b := range brokers {
			if broker, ok := b.(string); ok {
				cfg.Brokers[i] = broker
			} else {
				return fmt.Errorf("invalid broker type at index %d", i)
			}
		}
	} else if brokers, ok := config["brokers"].([]string); ok {
		cfg.Brokers = brokers
	} else {
		return fmt.Errorf("brokers is required")
	}

	// Topic routing: topic and topic_prefix are mutually exclusive (ADR-027)
	hasTopic := false
	if topic, ok := config["topic"].(string); ok && topic != "" {
		cfg.Topic = topic
		hasTopic = true
	}
	if prefix, ok := config["topic_prefix"].(string); ok && prefix != "" {
		cfg.TopicPrefix = prefix
		if hasTopic {
			return fmt.Errorf("topic and topic_prefix are mutually exclusive")
		}
	}
	if cfg.Topic == "" && cfg.TopicPrefix == "" {
		return fmt.Errorf("either topic or topic_prefix is required")
	}

	// Optional: batch_size
	if batchSize, ok := config["batch_size"].(float64); ok {
		cfg.BatchSize = int(batchSize)
	}

	// Optional: batch_timeout (string duration)
	if batchTimeout, ok := config["batch_timeout"].(string); ok {
		timeout, err := time.ParseDuration(batchTimeout)
		if err != nil {
			return fmt.Errorf("invalid batch_timeout: %w", err)
		}
		cfg.BatchTimeout = timeout
	}

	// Optional: compression
	if compression, ok := config["compression"].(string); ok {
		cfg.Compression = compression
	}

	// Optional: max_attempts
	if maxAttempts, ok := config["max_attempts"].(float64); ok {
		cfg.MaxAttempts = int(maxAttempts)
	}

	// Optional: serialization (ADR-028)
	if ser, ok := config["serialization"].(string); ok {
		switch ser {
		case "json", "binary":
			cfg.Serialization = ser
		default:
			return fmt.Errorf("invalid serialization: %s (must be json or binary)", ser)
		}
	}

	r.config = cfg

	// Create Kafka writer.
	// Topic is always set per-message in Report()/ReportBatch() via resolveTopic() (ADR-027).
	// Setting a topic on the writer AND on the message simultaneously is rejected by kafka-go,
	// so we never set writer-level Topic — resolveTopic() handles both fixed and prefix modes.
	writerConfig := kafka.WriterConfig{
		Brokers:      cfg.Brokers,
		Balancer:     &kafka.Hash{},
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		MaxAttempts:  cfg.MaxAttempts,
		Async:        false,
	}

	// Compression codec
	switch cfg.Compression {
	case "none", "":
		writerConfig.CompressionCodec = nil
	case "gzip":
		writerConfig.CompressionCodec = compress.Gzip.Codec()
	case "snappy":
		writerConfig.CompressionCodec = compress.Snappy.Codec()
	case "lz4":
		writerConfig.CompressionCodec = compress.Lz4.Codec()
	default:
		return fmt.Errorf("invalid compression type: %s", cfg.Compression)
	}

	r.writer = kafka.NewWriter(writerConfig)

	return nil
}

// Start starts the reporter.
func (r *KafkaReporter) Start(ctx context.Context) error {
	topicInfo := r.config.Topic
	if r.config.TopicPrefix != "" {
		topicInfo = r.config.TopicPrefix + "-{protocol}"
	}
	slog.Info("kafka reporter started",
		"brokers", r.config.Brokers,
		"topic", topicInfo,
		"batch_size", r.config.BatchSize,
		"batch_timeout", r.config.BatchTimeout,
		"compression", r.config.Compression,
		"serialization", r.config.Serialization,
	)
	return nil
}

// Stop stops the reporter.
func (r *KafkaReporter) Stop(ctx context.Context) error {
	if r.writer != nil {
		if err := r.writer.Close(); err != nil {
			slog.Error("error closing kafka writer", "error", err)
			return err
		}
	}

	reported := r.reportedCount.Load()
	errors := r.errorCount.Load()
	slog.Info("kafka reporter stopped",
		"total_reported", reported,
		"total_errors", errors,
	)
	return nil
}

// Report sends a packet to Kafka.
// Envelope metadata is placed in Kafka Headers, payload data in Value (ADR-028).
func (r *KafkaReporter) Report(ctx context.Context, pkt *core.OutputPacket) error {
	if pkt == nil {
		return fmt.Errorf("nil packet")
	}

	// Serialize payload to Value
	value, err := r.serializeValue(pkt)
	if err != nil {
		r.errorCount.Add(1)
		return fmt.Errorf("serialize packet failed: %w", err)
	}

	// Build Kafka message with envelope as Headers (ADR-028)
	msg := kafka.Message{
		Topic: r.resolveTopic(pkt),
		Key:   []byte(fmt.Sprintf("%s:%d-%s:%d", pkt.SrcIP, pkt.SrcPort, pkt.DstIP, pkt.DstPort)),
		Value: value,
		Time:  pkt.Timestamp,
	}

	// Envelope → Kafka Headers
	msg.Headers = r.buildHeaders(pkt)

	// Send to Kafka
	err = r.writer.WriteMessages(ctx, msg)
	if err != nil {
		r.errorCount.Add(1)
		return fmt.Errorf("kafka write failed: %w", err)
	}

	r.reportedCount.Add(1)
	return nil
}

// resolveTopic returns the target topic for a packet (ADR-027).
// With topic_prefix: "{prefix}-{protocol}" (e.g. "capture-agent-sip", "capture-agent-rtp").
// With fixed topic: returns the configured topic directly.
func (r *KafkaReporter) resolveTopic(pkt *core.OutputPacket) string {
	if r.config.TopicPrefix != "" {
		proto := pkt.PayloadType
		if proto == "" {
			proto = defaultProtocolFallback
		}
		return r.config.TopicPrefix + "-" + proto
	}
	return r.config.Topic
}

// buildHeaders creates Kafka headers from packet envelope metadata (ADR-028).
// Envelope fields (task_id, agent_id, network context) go into headers so
// Kafka Streams / consumers can filter without deserializing the value.
func (r *KafkaReporter) buildHeaders(pkt *core.OutputPacket) []kafka.Header {
	headers := make([]kafka.Header, 0, 8+len(pkt.Labels))

	// Core envelope
	headers = append(headers,
		kafka.Header{Key: "task_id", Value: []byte(pkt.TaskID)},
		kafka.Header{Key: "agent_id", Value: []byte(pkt.AgentID)},
		kafka.Header{Key: "payload_type", Value: []byte(pkt.PayloadType)},
		kafka.Header{Key: "src_ip", Value: []byte(pkt.SrcIP.String())},
		kafka.Header{Key: "dst_ip", Value: []byte(pkt.DstIP.String())},
		kafka.Header{Key: "src_port", Value: []byte(strconv.FormatUint(uint64(pkt.SrcPort), 10))},
		kafka.Header{Key: "dst_port", Value: []byte(strconv.FormatUint(uint64(pkt.DstPort), 10))},
		kafka.Header{Key: "timestamp", Value: []byte(strconv.FormatInt(pkt.Timestamp.UnixMilli(), 10))},
	)

	// Labels → headers with "l." prefix to avoid key collision
	for k, v := range pkt.Labels {
		headers = append(headers, kafka.Header{Key: "l." + k, Value: []byte(v)})
	}

	return headers
}

// serializeValue serializes the packet payload for the Kafka message value.
// Phase 1: JSON serialization. Phase 2: binary via Payload interface.
func (r *KafkaReporter) serializeValue(pkt *core.OutputPacket) ([]byte, error) {
	switch r.config.Serialization {
	case "json", "":
		return r.serializeJSON(pkt)
	case "binary":
		// Phase 2: when Payload implements MarshalBinary(), use it directly.
		// For now, fall back to JSON.
		return r.serializeJSON(pkt)
	default:
		return nil, fmt.Errorf("unsupported serialization: %s", r.config.Serialization)
	}
}

// serializeJSON converts OutputPacket payload to JSON bytes.
func (r *KafkaReporter) serializeJSON(pkt *core.OutputPacket) ([]byte, error) {
	output := map[string]any{
		"task_id":      pkt.TaskID,
		"agent_id":     pkt.AgentID,
		"pipeline_id":  pkt.PipelineID,
		"timestamp":    pkt.Timestamp.UnixMilli(),
		"src_ip":       pkt.SrcIP.String(),
		"dst_ip":       pkt.DstIP.String(),
		"src_port":     pkt.SrcPort,
		"dst_port":     pkt.DstPort,
		"protocol":     pkt.Protocol,
		"payload_type": pkt.PayloadType,
		"labels":       pkt.Labels,
	}

	// Typed payload (e.g. future structured types; SIP parser returns nil — labels carry
	// the SIP metadata, raw bytes are preserved below).
	if pkt.Payload != nil {
		output["payload"] = pkt.Payload
	}

	// Raw application-layer bytes, base64-encoded for JSON transport.
	if len(pkt.RawPayload) > 0 {
		output["raw_payload"] = base64.StdEncoding.EncodeToString(pkt.RawPayload)
		output["raw_payload_len"] = len(pkt.RawPayload)
	}

	return json.Marshal(output)
}

// Flush forces any pending messages to be sent.
func (r *KafkaReporter) Flush(ctx context.Context) error {
	return nil
}

// ReportBatch sends a batch of packets to Kafka in a single WriteMessages call.
// Implements plugin.BatchReporter for high-throughput batched delivery via ReporterWrapper.
func (r *KafkaReporter) ReportBatch(ctx context.Context, pkts []*core.OutputPacket) error {
	msgs := make([]kafka.Message, 0, len(pkts))
	for _, pkt := range pkts {
		if pkt == nil {
			continue
		}

		value, err := r.serializeValue(pkt)
		if err != nil {
			r.errorCount.Add(1)
			slog.Debug("batch serialize skip", "error", err)
			continue
		}

		msgs = append(msgs, kafka.Message{
			Topic:   r.resolveTopic(pkt),
			Key:     []byte(fmt.Sprintf("%s:%d-%s:%d", pkt.SrcIP, pkt.SrcPort, pkt.DstIP, pkt.DstPort)),
			Value:   value,
			Time:    pkt.Timestamp,
			Headers: r.buildHeaders(pkt),
		})
	}

	if len(msgs) == 0 {
		return nil
	}

	if err := r.writer.WriteMessages(ctx, msgs...); err != nil {
		r.errorCount.Add(uint64(len(msgs)))
		return fmt.Errorf("kafka batch write failed (%d msgs): %w", len(msgs), err)
	}

	r.reportedCount.Add(uint64(len(msgs)))
	return nil
}
