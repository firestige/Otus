package command

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/task"
)

// helper: minimal valid CommandChannelConfig for constructor tests
func validCCConfig() config.CommandChannelConfig {
	return config.CommandChannelConfig{
		Enabled:    true,
		Type:       "kafka",
		CommandTTL: "5m",
		Kafka: config.CommandKafkaConfig{
			Brokers:         []string{"localhost:9092"},
			Topic:           "commands",
			GroupID:         "otus-group",
			AutoOffsetReset: "latest",
		},
	}
}

func TestNewKafkaCommandConsumer(t *testing.T) {
	tm := task.NewTaskManager("test-agent")
	handler := NewCommandHandler(tm, nil)

	tests := []struct {
		name    string
		config  config.CommandChannelConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  validCCConfig(),
			wantErr: false,
		},
		{
			name: "missing brokers",
			config: config.CommandChannelConfig{
				Kafka: config.CommandKafkaConfig{
					Topic:   "commands",
					GroupID: "otus-group",
				},
			},
			wantErr: true,
		},
		{
			name: "missing topic",
			config: config.CommandChannelConfig{
				Kafka: config.CommandKafkaConfig{
					Brokers: []string{"localhost:9092"},
					GroupID: "otus-group",
				},
			},
			wantErr: true,
		},
		{
			name: "missing group_id",
			config: config.CommandChannelConfig{
				Kafka: config.CommandKafkaConfig{
					Brokers: []string{"localhost:9092"},
					Topic:   "commands",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consumer, err := NewKafkaCommandConsumer(tt.config, "test-node", handler)
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

func TestKafkaCommandConsumer_TTLParsing(t *testing.T) {
	tm := task.NewTaskManager("test-agent")
	handler := NewCommandHandler(tm, nil)

	cc := validCCConfig()
	cc.CommandTTL = "10m"

	consumer, err := NewKafkaCommandConsumer(cc, "test-node", handler)
	if err != nil {
		t.Fatalf("NewKafkaCommandConsumer() failed: %v", err)
	}
	defer consumer.Stop()

	if consumer.ttl != 10*time.Minute {
		t.Errorf("ttl = %v, want 10m", consumer.ttl)
	}
}

func TestKafkaCommandConsumer_InvalidTTL(t *testing.T) {
	tm := task.NewTaskManager("test-agent")
	handler := NewCommandHandler(tm, nil)

	cc := validCCConfig()
	cc.CommandTTL = "not-a-duration"

	_, err := NewKafkaCommandConsumer(cc, "test-node", handler)
	if err == nil {
		t.Fatal("expected error for invalid TTL")
	}
}

func TestKafkaCommandConsumer_StartStop(t *testing.T) {
	tm := task.NewTaskManager("test-agent")
	handler := NewCommandHandler(tm, nil)

	consumer, err := NewKafkaCommandConsumer(validCCConfig(), "test-node", handler)
	if err != nil {
		t.Fatalf("NewKafkaCommandConsumer() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- consumer.Start(ctx)
	}()

	select {
	case err := <-errChan:
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Logf("Start() returned: %v (acceptable)", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Start() didn't return after context cancellation")
	}

	if err := consumer.Stop(); err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

// ── processMessage unit tests (ADR-026) ──

func newTestConsumer(t *testing.T, hostname string) *KafkaCommandConsumer {
	t.Helper()
	tm := task.NewTaskManager("test-agent")
	handler := NewCommandHandler(tm, nil)
	consumer, err := NewKafkaCommandConsumer(validCCConfig(), hostname, handler)
	if err != nil {
		t.Fatalf("NewKafkaCommandConsumer: %v", err)
	}
	t.Cleanup(func() { _ = consumer.Stop() })
	return consumer
}

func makeMsg(kCmd KafkaCommand) kafka.Message {
	data, _ := json.Marshal(kCmd)
	return kafka.Message{Value: data}
}

func TestProcessMessage_TargetMatch(t *testing.T) {
	c := newTestConsumer(t, "node-01")

	// Broadcast
	err := c.processMessage(context.Background(), makeMsg(KafkaCommand{
		Version:   "v1",
		Target:    "*",
		Command:   "task_list",
		Timestamp: time.Now(),
		RequestID: "r1",
	}))
	if err != nil {
		t.Errorf("broadcast should succeed: %v", err)
	}

	// Exact match
	err = c.processMessage(context.Background(), makeMsg(KafkaCommand{
		Version:   "v1",
		Target:    "node-01",
		Command:   "task_list",
		Timestamp: time.Now(),
		RequestID: "r2",
	}))
	if err != nil {
		t.Errorf("exact target match should succeed: %v", err)
	}
}

func TestProcessMessage_TargetMismatch(t *testing.T) {
	c := newTestConsumer(t, "node-01")

	// Different target → silently skip (no error)
	err := c.processMessage(context.Background(), makeMsg(KafkaCommand{
		Version:   "v1",
		Target:    "node-99",
		Command:   "task_list",
		Timestamp: time.Now(),
		RequestID: "r3",
	}))
	if err != nil {
		t.Errorf("target mismatch should be skipped without error: %v", err)
	}
}

func TestProcessMessage_StaleCommand(t *testing.T) {
	c := newTestConsumer(t, "node-01")
	c.ttl = 1 * time.Minute // short TTL for test

	// Stale command → silently skip
	err := c.processMessage(context.Background(), makeMsg(KafkaCommand{
		Version:   "v1",
		Target:    "*",
		Command:   "task_list",
		Timestamp: time.Now().Add(-10 * time.Minute), // 10 min ago
		RequestID: "r4",
	}))
	if err != nil {
		t.Errorf("stale command should be skipped without error: %v", err)
	}
}

func TestProcessMessage_FreshCommand(t *testing.T) {
	c := newTestConsumer(t, "node-01")

	err := c.processMessage(context.Background(), makeMsg(KafkaCommand{
		Version:   "v1",
		Target:    "*",
		Command:   "task_list",
		Timestamp: time.Now(),
		RequestID: "r5",
	}))
	if err != nil {
		t.Errorf("fresh broadcast task_list should succeed: %v", err)
	}
}

func TestProcessMessage_InvalidJSON(t *testing.T) {
	c := newTestConsumer(t, "node-01")

	err := c.processMessage(context.Background(), kafka.Message{Value: []byte("not-json")})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestKafkaCommand_Serialization(t *testing.T) {
	kCmd := KafkaCommand{
		Version:   "v1",
		Target:    "node-01",
		Command:   "task_create",
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		RequestID: "req-abc-123",
		Payload:   json.RawMessage(`{"task_id":"t1"}`),
	}

	data, err := json.Marshal(kCmd)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded KafkaCommand
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Version != "v1" {
		t.Errorf("Version = %q", decoded.Version)
	}
	if decoded.Target != "node-01" {
		t.Errorf("Target = %q", decoded.Target)
	}
	if decoded.Command != "task_create" {
		t.Errorf("Command = %q", decoded.Command)
	}
	if decoded.RequestID != "req-abc-123" {
		t.Errorf("RequestID = %q", decoded.RequestID)
	}
}
