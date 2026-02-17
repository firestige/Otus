package console

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"firestige.xyz/otus/internal/core"
)

func TestConsoleReporter_Init(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		wantErr bool
		wantFmt string
	}{
		{
			name:    "nil config defaults to text",
			config:  nil,
			wantErr: false,
			wantFmt: "text",
		},
		{
			name:    "empty config defaults to text",
			config:  map[string]any{},
			wantErr: false,
			wantFmt: "text",
		},
		{
			name:    "json format",
			config:  map[string]any{"format": "json"},
			wantErr: false,
			wantFmt: "json",
		},
		{
			name:    "text format",
			config:  map[string]any{"format": "text"},
			wantErr: false,
			wantFmt: "text",
		},
		{
			name:    "invalid format",
			config:  map[string]any{"format": "xml"},
			wantErr: true,
			wantFmt: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewConsoleReporter().(*ConsoleReporter)
			err := r.Init(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && r.format != tt.wantFmt {
				t.Errorf("Init() format = %v, want %v", r.format, tt.wantFmt)
			}
		})
	}
}

func TestConsoleReporter_Report(t *testing.T) {
	r := NewConsoleReporter().(*ConsoleReporter)
	err := r.Init(map[string]any{"format": "json"})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	ctx := context.Background()
	err = r.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Create test packet
	pkt := &core.OutputPacket{
		TaskID:      "test-task",
		AgentID:     "test-agent",
		PipelineID:  1,
		Timestamp:   time.Now(),
		SrcIP:       netip.MustParseAddr("192.168.1.1"),
		DstIP:       netip.MustParseAddr("192.168.1.2"),
		SrcPort:     5060,
		DstPort:     5061,
		Protocol:    17, // UDP
		PayloadType: "sip",
		Labels: map[string]string{
			"sip.method":  "INVITE",
			"sip.call_id": "abc123",
		},
		RawPayload: []byte("test payload"),
	}

	// Report should not fail
	err = r.Report(ctx, pkt)
	if err != nil {
		t.Errorf("Report() error = %v", err)
	}

	// Check counter
	if count := r.reportedCount.Load(); count != 1 {
		t.Errorf("reportedCount = %d, want 1", count)
	}

	// Test nil packet
	err = r.Report(ctx, nil)
	if err == nil {
		t.Error("Report(nil) should return error")
	}

	err = r.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestConsoleReporter_TextFormat(t *testing.T) {
	r := NewConsoleReporter().(*ConsoleReporter)
	err := r.Init(map[string]any{"format": "text"})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	pkt := &core.OutputPacket{
		TaskID:      "test-task",
		Timestamp:   time.Now(),
		SrcIP:       netip.MustParseAddr("10.0.0.1"),
		DstIP:       netip.MustParseAddr("10.0.0.2"),
		SrcPort:     1234,
		DstPort:     5678,
		Protocol:    6, // TCP
		PayloadType: "http",
	}

	// This will output to stdout but should not error
	err = r.Report(context.Background(), pkt)
	if err != nil {
		t.Errorf("Report() error = %v", err)
	}
}

func TestConsoleReporter_Lifecycle(t *testing.T) {
	r := NewConsoleReporter()

	if name := r.Name(); name != "console" {
		t.Errorf("Name() = %s, want console", name)
	}

	ctx := context.Background()

	err := r.Start(ctx)
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
