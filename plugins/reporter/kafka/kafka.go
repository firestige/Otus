// Package kafka implements Kafka reporter plugin.
// Sends OutputPackets to Kafka with batching, compression, and retry support.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/compress"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/pkg/plugin"
)

const (
	defaultBatchSize    = 100
	defaultBatchTimeout = 100 * time.Millisecond
	defaultCompression  = "snappy"
	defaultMaxAttempts  = 3
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
	Brokers      []string      `json:"brokers"`       // required
	Topic        string        `json:"topic"`         // required
	BatchSize    int           `json:"batch_size"`    // optional, default 100
	BatchTimeout time.Duration `json:"batch_timeout"` // optional, default 100ms
	Compression  string        `json:"compression"`   // optional: none|gzip|snappy|lz4, default snappy
	MaxAttempts  int           `json:"max_attempts"`  // optional, default 3
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

	// Parse configuration
	cfg := Config{
		BatchSize:    defaultBatchSize,
		BatchTimeout: defaultBatchTimeout,
		Compression:  defaultCompression,
		MaxAttempts:  defaultMaxAttempts,
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
	} else {
		return fmt.Errorf("brokers is required")
	}

	// Required: topic
	if topic, ok := config["topic"].(string); ok {
		cfg.Topic = topic
	} else {
		return fmt.Errorf("topic is required")
	}

	// Optional: batch_size
	if batchSize, ok := config["batch_size"].(float64); ok {
		cfg.BatchSize = int(batchSize)
	}

	// Optional: batch_timeout (can be string or duration)
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

	r.config = cfg

	// Create Kafka writer
	writerConfig := kafka.WriterConfig{
		Brokers:      cfg.Brokers,
		Topic:        cfg.Topic,
		Balancer:     &kafka.Hash{}, // Use hash balancer for consistent routing
		BatchSize:    cfg.BatchSize,
		BatchTimeout: cfg.BatchTimeout,
		MaxAttempts:  cfg.MaxAttempts,
		Async:        false, // Synchronous for error handling
	}

	// Set compression codec
	switch cfg.Compression {
	case "none", "":
		writerConfig.CompressionCodec = nil // No compression
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
	slog.Info("kafka reporter started",
		"brokers", r.config.Brokers,
		"topic", r.config.Topic,
		"batch_size", r.config.BatchSize,
		"batch_timeout", r.config.BatchTimeout,
		"compression", r.config.Compression,
	)
	return nil
}

// Stop stops the reporter.
func (r *KafkaReporter) Stop(ctx context.Context) error {
	if r.writer != nil {
		// Flush any pending messages
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
func (r *KafkaReporter) Report(ctx context.Context, pkt *core.OutputPacket) error {
	if pkt == nil {
		return fmt.Errorf("nil packet")
	}

	// Serialize packet to JSON
	value, err := r.serializePacket(pkt)
	if err != nil {
		r.errorCount.Add(1)
		return fmt.Errorf("serialize packet failed: %w", err)
	}

	// Create Kafka message
	msg := kafka.Message{
		Key:   []byte(fmt.Sprintf("%s:%d-%s:%d", pkt.SrcIP, pkt.SrcPort, pkt.DstIP, pkt.DstPort)),
		Value: value,
		Time:  pkt.Timestamp,
	}

	// Add labels as Kafka headers
	if len(pkt.Labels) > 0 {
		msg.Headers = make([]kafka.Header, 0, len(pkt.Labels))
		for k, v := range pkt.Labels {
			msg.Headers = append(msg.Headers, kafka.Header{
				Key:   k,
				Value: []byte(v),
			})
		}
	}

	// Send to Kafka
	err = r.writer.WriteMessages(ctx, msg)
	if err != nil {
		r.errorCount.Add(1)
		return fmt.Errorf("kafka write failed: %w", err)
	}

	r.reportedCount.Add(1)
	return nil
}

// serializePacket converts OutputPacket to JSON bytes.
func (r *KafkaReporter) serializePacket(pkt *core.OutputPacket) ([]byte, error) {
	// Create a JSON-serializable representation
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

	// Include raw payload as base64 if present
	if len(pkt.RawPayload) > 0 {
		output["raw_payload_len"] = len(pkt.RawPayload)
		// Note: For production, you might want to base64 encode the raw payload
		// or send it separately. For now, we only include the length.
	}

	return json.Marshal(output)
}

// Flush forces any pending messages to be sent.
func (r *KafkaReporter) Flush(ctx context.Context) error {
	// kafka.Writer automatically batches and flushes based on BatchSize/BatchTimeout
	// No explicit flush needed, but we can close and reopen if needed
	return nil
}
