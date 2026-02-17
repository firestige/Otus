// Package command implements command channels.
package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

// KafkaCommandConfig represents Kafka command consumer configuration.
type KafkaCommandConfig struct {
	Brokers      []string      `json:"brokers"`       // Kafka brokers
	Topic        string        `json:"topic"`         // Command topic
	GroupID      string        `json:"group_id"`      // Consumer group ID
	StartOffset  string        `json:"start_offset"`  // "earliest" or "latest", default "latest"
	PollInterval time.Duration `json:"poll_interval"` // Poll interval, default 1s
	MaxRetries   int           `json:"max_retries"`   // Max retries for processing errors, default 3
}

// KafkaCommandConsumer consumes commands from Kafka and dispatches to handler.
type KafkaCommandConsumer struct {
	config  KafkaCommandConfig
	reader  *kafka.Reader
	handler *CommandHandler
}

// NewKafkaCommandConsumer creates a new Kafka command consumer.
func NewKafkaCommandConsumer(config KafkaCommandConfig, handler *CommandHandler) (*KafkaCommandConsumer, error) {
	if len(config.Brokers) == 0 {
		return nil, fmt.Errorf("brokers is required")
	}
	if config.Topic == "" {
		return nil, fmt.Errorf("topic is required")
	}
	if config.GroupID == "" {
		return nil, fmt.Errorf("group_id is required")
	}

	// Set defaults
	if config.StartOffset == "" {
		config.StartOffset = "latest"
	}
	if config.PollInterval == 0 {
		config.PollInterval = 1 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	// Determine start offset
	var startOffset int64
	switch config.StartOffset {
	case "earliest":
		startOffset = kafka.FirstOffset
	case "latest":
		startOffset = kafka.LastOffset
	default:
		return nil, fmt.Errorf("invalid start_offset %q, must be 'earliest' or 'latest'", config.StartOffset)
	}

	// Create Kafka reader (consumer)
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        config.Brokers,
		Topic:          config.Topic,
		GroupID:        config.GroupID,
		StartOffset:    startOffset,
		MinBytes:       1,           // Fetch as soon as 1 byte available
		MaxBytes:       10 << 20,    // 10MB max
		CommitInterval: time.Second, // Auto-commit every second
		MaxWait:        config.PollInterval,
	})

	return &KafkaCommandConsumer{
		config:  config,
		reader:  reader,
		handler: handler,
	}, nil
}

// Start starts consuming commands from Kafka.
// Blocks until context is cancelled or an unrecoverable error occurs.
func (c *KafkaCommandConsumer) Start(ctx context.Context) error {
	slog.Info("kafka command consumer started",
		"brokers", c.config.Brokers,
		"topic", c.config.Topic,
		"group_id", c.config.GroupID,
		"start_offset", c.config.StartOffset,
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
			// Wait a bit before retrying
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
			// Continue processing even if one message fails
		}

		// Commit the message
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("failed to commit message", "error", err)
		}
	}
}

// processMessage processes a single Kafka message as a command.
func (c *KafkaCommandConsumer) processMessage(ctx context.Context, msg kafka.Message) error {
	// Parse command from JSON
	var cmd Command
	if err := json.Unmarshal(msg.Value, &cmd); err != nil {
		return fmt.Errorf("failed to parse command: %w", err)
	}

	// Log the received command
	slog.Info("received command",
		"method", cmd.Method,
		"id", cmd.ID,
		"partition", msg.Partition,
		"offset", msg.Offset,
	)

	// Handle the command
	response := c.handler.Handle(ctx, cmd)

	// Log the response
	if response.Error != nil {
		slog.Error("command execution failed",
			"method", cmd.Method,
			"id", cmd.ID,
			"error_code", response.Error.Code,
			"error_message", response.Error.Message,
		)
		return fmt.Errorf("command failed: %s", response.Error.Message)
	}

	slog.Info("command executed successfully",
		"method", cmd.Method,
		"id", cmd.ID,
		"result", response.Result,
	)

	return nil
}

// Stop stops the Kafka consumer and closes the connection.
func (c *KafkaCommandConsumer) Stop() error {
	if c.reader != nil {
		slog.Info("closing kafka command consumer")
		if err := c.reader.Close(); err != nil {
			return fmt.Errorf("failed to close kafka reader: %w", err)
		}
	}
	return nil
}
