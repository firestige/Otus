// Package command implements command channels.
package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"icc.tech/capture-agent/internal/config"
)

// KafkaCommand is the wire format for commands received via Kafka (ADR-026).
//
// Example JSON:
//
//	{
//	  "version":    "v1",
//	  "target":     "node-01",
//	  "command":    "task_create",
//	  "timestamp":  "2024-01-15T10:30:00Z",
//	  "request_id": "req-abc-123",
//	  "payload":    { ... }
//	}
type KafkaCommand struct {
	Version   string          `json:"version"`    // Protocol version ("v1")
	Target    string          `json:"target"`     // Node hostname or "*" for broadcast
	Command   string          `json:"command"`    // Command name (e.g., "task_create")
	Timestamp time.Time       `json:"timestamp"`  // When the command was issued
	RequestID string          `json:"request_id"` // Unique request ID for tracing
	Payload   json.RawMessage `json:"payload"`    // Command-specific parameters
}

// SimpleCommand is the simplified Kafka wire format for start/stop operations.
// Each agent consumes all messages and self-selects items matching its local role.
//
// Example JSON:
//
//	{
//	  "id":        "a1b2c3d4e5f6",
//	  "timestamp": 1709000000000,
//	  "data": [
//	    { "role": "FS", "cmd": "START", "portRange": "10000-60000", "protocol": ["SIP","RTP"] }
//	  ]
//	}
type SimpleCommand struct {
	ID        string          `json:"id"`        // UUID without dashes
	Timestamp int64           `json:"timestamp"` // Unix ms
	Data      []SimpleCmdItem `json:"data"`
}

// SimpleCmdItem is one element of SimpleCommand.Data.
type SimpleCmdItem struct {
	Role      string   `json:"role"`                // ASBC | FS | KAMAILIO | TRACEMEDIA
	Cmd       string   `json:"cmd"`                 // START | STOP
	PortRange *string  `json:"portRange,omitempty"` // "min-max"; nil = role default
	Protocol  []string `json:"protocol,omitempty"`  // ["SIP","RTP"]; nil = role default
}

// messageWriter abstracts kafka.Writer for testability.
type messageWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// KafkaResponse is the wire format for command responses written to the response topic (ADR-029).
//
// Example JSON:
//
//	{
//	  "version":    "v1",
//	  "source":     "edge-beijing-01",
//	  "command":    "task_list",
//	  "request_id": "req-abc-123",
//	  "timestamp":  "2026-02-21T10:30:00Z",
//	  "result":     { ... }
//	}
type KafkaResponse struct {
	Version   string      `json:"version"`          // Protocol version ("v1")
	Source    string      `json:"source"`           // Agent hostname (the responder)
	Command   string      `json:"command"`          // Echoed from KafkaCommand
	RequestID string      `json:"request_id"`       // Correlation ID (echoed from KafkaCommand)
	Timestamp time.Time   `json:"timestamp"`        // When the response was produced
	Result    interface{} `json:"result,omitempty"` // Command result, nil on error
	Error     *ErrorInfo  `json:"error,omitempty"`  // Non-nil when command failed
}

// KafkaCommandConsumer consumes commands from Kafka and dispatches to handler.
type KafkaCommandConsumer struct {
	ccConfig  config.CommandChannelConfig
	hostname  string // local node hostname for target matching (legacy KafkaCommand)
	agentRole string // local agent role for SimpleCommand routing
	reader    *kafka.Reader
	writer    messageWriter // nil when response_topic is empty (ADR-029)
	handler   *CommandHandler
	ttl       time.Duration // command TTL for stale-command rejection
}

// NewKafkaCommandConsumer creates a new Kafka command consumer using the global config.
func NewKafkaCommandConsumer(ccConfig config.CommandChannelConfig, hostname, agentRole string, handler *CommandHandler) (*KafkaCommandConsumer, error) {
	kc := ccConfig.Kafka
	if len(kc.Brokers) == 0 {
		return nil, fmt.Errorf("brokers is required")
	}
	if kc.Topic == "" {
		return nil, fmt.Errorf("topic is required")
	}
	if kc.GroupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}

	// Parse command TTL
	ttl := 5 * time.Minute // default
	if ccConfig.CommandTTL != "" {
		var err error
		ttl, err = time.ParseDuration(ccConfig.CommandTTL)
		if err != nil {
			return nil, fmt.Errorf("invalid command_ttl %q: %w", ccConfig.CommandTTL, err)
		}
	}

	// Determine start offset
	var startOffset int64
	switch kc.AutoOffsetReset {
	case "earliest":
		startOffset = kafka.FirstOffset
	default:
		startOffset = kafka.LastOffset
	}

	// Create Kafka reader (consumer)
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        kc.Brokers,
		Topic:          kc.Topic,
		GroupID:        kc.GroupID,
		StartOffset:    startOffset,
		MinBytes:       1,
		MaxBytes:       10 << 20,
		CommitInterval: time.Second,
		MaxWait:        1 * time.Second,
	})

	// Create Kafka writer (producer) for response channel — only when response_topic is set (ADR-029)
	var writer messageWriter
	if kc.ResponseTopic != "" {
		writer = &kafka.Writer{
			Addr:         kafka.TCP(kc.Brokers...),
			Topic:        kc.ResponseTopic,
			Balancer:     &kafka.Hash{}, // hostname as key → consistent partition routing
			RequiredAcks: kafka.RequireOne,
			Async:        false, // synchronous write so failures are observable
		}
	}

	return &KafkaCommandConsumer{
		ccConfig:  ccConfig,
		hostname:  hostname,
		agentRole: agentRole,
		reader:    reader,
		writer:    writer,
		handler:   handler,
		ttl:       ttl,
	}, nil
}

// Start starts consuming commands from Kafka.
// Blocks until context is cancelled or an unrecoverable error occurs.
func (c *KafkaCommandConsumer) Start(ctx context.Context) error {
	slog.Info("kafka command consumer started",
		"brokers", c.ccConfig.Kafka.Brokers,
		"topic", c.ccConfig.Kafka.Topic,
		"group_id", c.ccConfig.Kafka.GroupID,
		"hostname", c.hostname,
		"ttl", c.ttl,
	)

	for {
		select {
		case <-ctx.Done():
			slog.Info("kafka command consumer stopped", "reason", ctx.Err())
			return ctx.Err()
		default:
		}

		// Fetch message with context
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				return err
			}
			slog.Error("failed to fetch kafka message", "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// Process the message
		if err := c.processMessage(ctx, msg); err != nil {
			slog.Error("failed to process command",
				"error", err,
				"topic", msg.Topic,
				"partition", msg.Partition,
				"offset", msg.Offset,
			)
		}

		// Commit the message
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("failed to commit message", "error", err)
		}
	}
}

// processMessage processes a single Kafka message.
// It first attempts to detect a SimpleCommand (has "data" array field).
// If that fails or the message format doesn't match, it falls back to the
// legacy KafkaCommand format (ADR-026).
func (c *KafkaCommandConsumer) processMessage(ctx context.Context, msg kafka.Message) error {
	// Probe message format by checking for the "data" field.
	var probe struct {
		Data    json.RawMessage `json:"data"`
		Command string          `json:"command"`
	}
	if err := json.Unmarshal(msg.Value, &probe); err != nil {
		return fmt.Errorf("failed to parse kafka message: %w", err)
	}

	// SimpleCommand path: "data" field is a non-null JSON array.
	if len(probe.Data) > 0 && probe.Data[0] == '[' {
		return c.processSimpleCommand(ctx, msg.Value)
	}

	// Legacy KafkaCommand path.
	return c.processKafkaCommand(ctx, msg)
}

// processSimpleCommand handles the new SimpleCommand format.
// Each item in data[] is matched against the agent's local role; non-matching
// items are silently skipped.
func (c *KafkaCommandConsumer) processSimpleCommand(ctx context.Context, raw []byte) error {
	if c.agentRole == "" {
		slog.Debug("agent role not configured, skipping SimpleCommand")
		return nil
	}

	var sc SimpleCommand
	if err := json.Unmarshal(raw, &sc); err != nil {
		return fmt.Errorf("failed to parse SimpleCommand: %w", err)
	}

	slog.Info("received SimpleCommand", "id", sc.ID, "items", len(sc.Data))

	matched := false
	for i, item := range sc.Data {
		if !strings.EqualFold(item.Role, c.agentRole) {
			continue
		}
		if matched {
			slog.Warn("duplicate role in SimpleCommand data, ignoring",
				"id", sc.ID, "role", item.Role, "index", i)
			continue
		}
		matched = true

		// Map cmd → internal method name.
		var method string
		switch strings.ToUpper(item.Cmd) {
		case "START":
			method = "task_start"
		case "STOP":
			method = "task_stop"
		default:
			slog.Warn("unknown SimpleCommand cmd", "cmd", item.Cmd, "id", sc.ID)
			continue
		}

		// Serialize the item as params so handler can reconstruct it.
		params, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("marshal SimpleCmdItem: %w", err)
		}

		cmd := Command{Method: method, Params: params, ID: sc.ID}
		resp := c.handler.Handle(ctx, cmd)

		// Write response back to Kafka response topic (mirrors KafkaCommand behaviour).
		if c.writer != nil && sc.ID != "" {
			if err := c.writeResponse(ctx, method, resp); err != nil {
				slog.Error("failed to write SimpleCommand response",
					"id", sc.ID, "method", method, "error", err)
			}
		}

		if resp.Error != nil {
			slog.Error("SimpleCommand execution failed",
				"id", sc.ID, "method", method,
				"code", resp.Error.Code, "msg", resp.Error.Message)
			return fmt.Errorf("simple command failed: %s", resp.Error.Message)
		}
		slog.Info("SimpleCommand executed", "id", sc.ID, "method", method)
	}

	if !matched {
		slog.Debug("no matching role in SimpleCommand",
			"id", sc.ID, "agent_role", c.agentRole)
	}
	return nil
}

// processKafkaCommand handles the legacy KafkaCommand format (ADR-026).
func (c *KafkaCommandConsumer) processKafkaCommand(ctx context.Context, msg kafka.Message) error {
	// 1. Deserialize into KafkaCommand
	var kCmd KafkaCommand
	if err := json.Unmarshal(msg.Value, &kCmd); err != nil {
		return fmt.Errorf("failed to parse kafka command: %w", err)
	}

	// 2. Target filter: skip if not for this node and not broadcast
	if kCmd.Target != "*" && kCmd.Target != "" && kCmd.Target != c.hostname {
		slog.Debug("skipping command not targeting this node",
			"target", kCmd.Target,
			"hostname", c.hostname,
			"request_id", kCmd.RequestID,
		)
		return nil
	}

	// 3. Stale command check: reject commands older than TTL
	if !kCmd.Timestamp.IsZero() && time.Since(kCmd.Timestamp) > c.ttl {
		slog.Warn("skipping stale command",
			"command", kCmd.Command,
			"request_id", kCmd.RequestID,
			"timestamp", kCmd.Timestamp,
			"age", time.Since(kCmd.Timestamp),
			"ttl", c.ttl,
		)
		return nil
	}

	slog.Info("received kafka command",
		"command", kCmd.Command,
		"request_id", kCmd.RequestID,
		"target", kCmd.Target,
		"version", kCmd.Version,
	)

	// 4. Convert KafkaCommand → internal Command
	cmd := Command{
		Method: kCmd.Command,
		Params: kCmd.Payload,
		ID:     kCmd.RequestID,
	}

	// 5. Handle the command
	response := c.handler.Handle(ctx, cmd)

	// 6. Write response back to Kafka if response channel is configured (ADR-029).
	// We write even when the command failed so the caller learns the failure reason.
	if c.writer != nil && cmd.ID != "" {
		if err := c.writeResponse(ctx, kCmd.Command, response); err != nil {
			slog.Error("failed to write kafka response",
				"request_id", cmd.ID,
				"error", err,
			)
			// intentionally not returned: command already executed
		} else {
			slog.Debug("kafka response written",
				"request_id", cmd.ID,
				"source", c.hostname,
			)
		}
	}

	if response.Error != nil {
		slog.Error("command execution failed",
			"method", cmd.Method,
			"request_id", cmd.ID,
			"error_code", response.Error.Code,
			"error_message", response.Error.Message,
		)
		return fmt.Errorf("command failed: %s", response.Error.Message)
	}

	slog.Info("command executed successfully",
		"method", cmd.Method,
		"request_id", cmd.ID,
	)

	return nil
}

// writeResponse serialises response as KafkaResponse and publishes it to the response topic.
func (c *KafkaCommandConsumer) writeResponse(ctx context.Context, command string, resp Response) error {
	kr := KafkaResponse{
		Version:   "v1",
		Source:    c.hostname,
		Command:   command,
		RequestID: resp.ID,
		Timestamp: time.Now().UTC(),
		Result:    resp.Result,
		Error:     resp.Error,
	}
	data, err := json.Marshal(kr)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	return c.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(c.hostname), // consistent partition routing (hostname as key)
		Value: data,
	})
}

// Stop stops the Kafka consumer and closes the connection.
// Always nils the reader and writer to prevent double-close.
func (c *KafkaCommandConsumer) Stop() error {
	var errs []error

	if c.writer != nil {
		slog.Info("closing kafka response writer")
		writer := c.writer
		c.writer = nil
		if err := writer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close response writer: %w", err))
		}
	}

	if c.reader != nil {
		slog.Info("closing kafka command consumer")
		reader := c.reader
		c.reader = nil
		if err := reader.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close command reader: %w", err))
		}
	}

	return errors.Join(errs...)
}
