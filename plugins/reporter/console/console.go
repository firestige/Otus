// Package console implements console debug reporter.
// Outputs packets to stdout in human-readable format for debugging.
package console

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"

	"icc.tech/capture-agent/internal/core"
	"icc.tech/capture-agent/pkg/plugin"
)

// ConsoleReporter outputs packets to console for debugging.
type ConsoleReporter struct {
	name          string
	format        string // "json" or "text"
	reportedCount atomic.Uint64
}

// Config represents console reporter configuration.
type Config struct {
	Format string `json:"format"` // "json" or "text", default "text"
}

// NewConsoleReporter creates a new console reporter.
func NewConsoleReporter() plugin.Reporter {
	return &ConsoleReporter{
		name:   "console",
		format: "text", // default
	}
}

// Name returns the plugin name.
func (r *ConsoleReporter) Name() string {
	return r.name
}

// Init initializes the reporter with configuration.
func (r *ConsoleReporter) Init(config map[string]any) error {
	if config == nil {
		return nil
	}

	// Parse format if specified
	if format, ok := config["format"].(string); ok {
		if format != "json" && format != "text" {
			return fmt.Errorf("invalid format %q, must be json or text", format)
		}
		r.format = format
	}

	return nil
}

// Start starts the reporter.
func (r *ConsoleReporter) Start(ctx context.Context) error {
	slog.Info("console reporter started", "format", r.format)
	return nil
}

// Stop stops the reporter.
func (r *ConsoleReporter) Stop(ctx context.Context) error {
	count := r.reportedCount.Load()
	slog.Info("console reporter stopped", "total_reported", count)
	return nil
}

// Report outputs a packet to console.
func (r *ConsoleReporter) Report(ctx context.Context, pkt *core.OutputPacket) error {
	if pkt == nil {
		return fmt.Errorf("nil packet")
	}

	r.reportedCount.Add(1)

	if r.format == "json" {
		return r.reportJSON(pkt)
	}
	return r.reportText(pkt)
}

// reportJSON outputs packet in JSON format.
func (r *ConsoleReporter) reportJSON(pkt *core.OutputPacket) error {
	// Create a JSON-serializable representation
	output := map[string]any{
		"task_id":      pkt.TaskID,
		"agent_id":     pkt.AgentID,
		"pipeline_id":  pkt.PipelineID,
		"timestamp":    pkt.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		"src_ip":       pkt.SrcIP.String(),
		"dst_ip":       pkt.DstIP.String(),
		"src_port":     pkt.SrcPort,
		"dst_port":     pkt.DstPort,
		"protocol":     pkt.Protocol,
		"payload_type": pkt.PayloadType,
		"labels":       pkt.Labels,
	}

	// Add raw payload length if present
	if len(pkt.RawPayload) > 0 {
		output["raw_payload_len"] = len(pkt.RawPayload)
	}

	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("json marshal failed: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

// reportText outputs packet in human-readable text format.
func (r *ConsoleReporter) reportText(pkt *core.OutputPacket) error {
	fmt.Printf("[%s] %s:%d → %s:%d proto=%d type=%s",
		pkt.Timestamp.Format("15:04:05.000"),
		pkt.SrcIP, pkt.SrcPort,
		pkt.DstIP, pkt.DstPort,
		pkt.Protocol,
		pkt.PayloadType,
	)

	// Print labels if present
	if len(pkt.Labels) > 0 {
		fmt.Printf(" labels=%v", pkt.Labels)
	}

	// Print raw payload length
	if len(pkt.RawPayload) > 0 {
		fmt.Printf(" payload_len=%d", len(pkt.RawPayload))
	}

	fmt.Println()
	return nil
}

// Flush is a no-op for console reporter (stdout auto-flushes).
func (r *ConsoleReporter) Flush(ctx context.Context) error {
	return nil
}
