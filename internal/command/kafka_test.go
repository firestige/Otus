package command

import (
	"context"
	"testing"
	"time"

	"firestige.xyz/otus/internal/task"
)

func TestNewKafkaCommandConsumer(t *testing.T) {
	tm := task.NewTaskManager("test-agent")
	handler := NewCommandHandler(tm, nil)

	tests := []struct {
		name    string
		config  KafkaCommandConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: KafkaCommandConfig{
				Brokers: []string{"localhost:9092"},
				Topic:   "commands",
				GroupID: "otus-group",
			},
			wantErr: false,
		},
		{
			name: "missing brokers",
			config: KafkaCommandConfig{
				Topic:   "commands",
				GroupID: "otus-group",
			},
			wantErr: true,
		},
		{
			name: "missing topic",
			config: KafkaCommandConfig{
				Brokers: []string{"localhost:9092"},
				GroupID: "otus-group",
			},
			wantErr: true,
		},
		{
			name: "missing group_id",
			config: KafkaCommandConfig{
				Brokers: []string{"localhost:9092"},
				Topic:   "commands",
			},
			wantErr: true,
		},
		{
			name: "invalid start_offset",
			config: KafkaCommandConfig{
				Brokers:     []string{"localhost:9092"},
				Topic:       "commands",
				GroupID:     "otus-group",
				StartOffset: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consumer, err := NewKafkaCommandConsumer(tt.config, handler)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewKafkaCommandConsumer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && consumer == nil {
				t.Error("expected non-nil consumer")
			}
			if consumer != nil && consumer.reader != nil {
				_ = consumer.Stop()
			}
		})
	}
}

func TestKafkaCommandConsumer_ConfigDefaults(t *testing.T) {
	tm := task.NewTaskManager("test-agent")
	handler := NewCommandHandler(tm, nil)

	config := KafkaCommandConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "commands",
		GroupID: "otus-group",
	}

	consumer, err := NewKafkaCommandConsumer(config, handler)
	if err != nil {
		t.Fatalf("NewKafkaCommandConsumer() failed: %v", err)
	}
	defer consumer.Stop()

	// Check defaults
	if consumer.config.StartOffset != "latest" {
		t.Errorf("StartOffset = %s, want latest", consumer.config.StartOffset)
	}
	if consumer.config.PollInterval != 1*time.Second {
		t.Errorf("PollInterval = %v, want 1s", consumer.config.PollInterval)
	}
	if consumer.config.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", consumer.config.MaxRetries)
	}
}

func TestKafkaCommandConsumer_StartStop(t *testing.T) {
	tm := task.NewTaskManager("test-agent")
	handler := NewCommandHandler(tm, nil)

	config := KafkaCommandConfig{
		Brokers: []string{"localhost:9092"},
		Topic:   "test-commands",
		GroupID: "test-group",
	}

	consumer, err := NewKafkaCommandConsumer(config, handler)
	if err != nil {
		t.Fatalf("NewKafkaCommandConsumer() failed: %v", err)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start in goroutine (will block)
	errChan := make(chan error, 1)
	go func() {
		errChan <- consumer.Start(ctx)
	}()

	// Wait for context to expire or error
	select {
	case err := <-errChan:
		// Should return context.DeadlineExceeded or context.Canceled
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Logf("Start() returned: %v (acceptable)", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Start() didn't return after context cancellation")
	}

	// Stop the consumer
	err = consumer.Stop()
	if err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

func TestKafkaCommandConsumer_StartOffsets(t *testing.T) {
	tm := task.NewTaskManager("test-agent")
	handler := NewCommandHandler(tm, nil)

	offsets := []string{"earliest", "latest"}

	for _, offset := range offsets {
		t.Run(offset, func(t *testing.T) {
			config := KafkaCommandConfig{
				Brokers:     []string{"localhost:9092"},
				Topic:       "test-commands",
				GroupID:     "test-group",
				StartOffset: offset,
			}

			consumer, err := NewKafkaCommandConsumer(config, handler)
			if err != nil {
				t.Fatalf("NewKafkaCommandConsumer() failed: %v", err)
			}
			defer consumer.Stop()

			if consumer.config.StartOffset != offset {
				t.Errorf("StartOffset = %s, want %s", consumer.config.StartOffset, offset)
			}
		})
	}
}
