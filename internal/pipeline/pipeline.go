// Package pipeline implements the packet processing pipeline engine.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/internal/core/decoder"
	"firestige.xyz/otus/pkg/plugin"
)

// Pipeline represents a single-threaded packet processing chain.
type Pipeline struct {
	id       int
	taskID   string
	agentID  string
	capturer plugin.Capturer
	decoder  decoder.Decoder
	parsers  []plugin.Parser
	processors []plugin.Processor
	reporters []plugin.Reporter
	metrics  *Metrics
	
	// Runtime state
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	
	// Channel for backpressure control
	rawPacketChan chan core.RawPacket
}

// Config contains pipeline configuration.
type Config struct {
	ID          int
	TaskID      string
	AgentID     string
	Capturer    plugin.Capturer
	Decoder     decoder.Decoder
	Parsers     []plugin.Parser
	Processors  []plugin.Processor
	Reporters   []plugin.Reporter
	BufferSize  int // Raw packet channel buffer size
}

// New creates a new pipeline.
func New(cfg Config) *Pipeline {
	if cfg.BufferSize == 0 {
		cfg.BufferSize = 1024 // Default buffer size
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Pipeline{
		id:            cfg.ID,
		taskID:        cfg.TaskID,
		agentID:       cfg.AgentID,
		capturer:      cfg.Capturer,
		decoder:       cfg.Decoder,
		parsers:       cfg.Parsers,
		processors:    cfg.Processors,
		reporters:     cfg.Reporters,
		metrics:       NewMetrics(cfg.TaskID, cfg.ID),
		ctx:           ctx,
		cancel:        cancel,
		rawPacketChan: make(chan core.RawPacket, cfg.BufferSize),
	}
}

// Start starts the pipeline processing.
func (p *Pipeline) Start() error {
	slog.Info("pipeline starting", "task_id", p.taskID, "pipeline_id", p.id)
	
	// Start capture goroutine
	p.wg.Add(1)
	go p.captureLoop()
	
	// Start processing goroutine
	p.wg.Add(1)
	go p.processLoop()
	
	return nil
}

// Stop stops the pipeline gracefully.
func (p *Pipeline) Stop() error {
	slog.Info("pipeline stopping", "task_id", p.taskID, "pipeline_id", p.id)
	
	// Cancel context to signal goroutines to stop
	p.cancel()
	
	// Wait for all goroutines to finish
	p.wg.Wait()
	
	// Flush reporters
	for _, reporter := range p.reporters {
		if err := reporter.Flush(context.Background()); err != nil {
			slog.Error("reporter flush failed", "error", err)
		}
	}
	
	slog.Info("pipeline stopped", "task_id", p.taskID, "pipeline_id", p.id)
	return nil
}

// captureLoop reads packets from capturer and sends to processing channel.
func (p *Pipeline) captureLoop() {
	defer p.wg.Done()
	
	// Start capturer
	if err := p.capturer.Capture(p.ctx, p.rawPacketChan); err != nil {
		if p.ctx.Err() == nil {
			// Context not cancelled, this is a real error
			slog.Error("capture failed", "error", err, "task_id", p.taskID, "pipeline_id", p.id)
		}
	}
	
	// Close channel when capture ends
	close(p.rawPacketChan)
}

// processLoop is the main processing loop.
func (p *Pipeline) processLoop() {
	defer p.wg.Done()
	
	for {
		select {
		case <-p.ctx.Done():
			return
			
		case raw, ok := <-p.rawPacketChan:
			if !ok {
				// Channel closed, capturer stopped
				return
			}
			
			p.metrics.Received.Add(1)
			
			// Process packet (all steps are synchronous function calls)
			if err := p.processPacket(raw); err != nil {
				// Log error but continue processing
				slog.Debug("packet processing failed", "error", err)
			}
		}
	}
}

// processPacket processes a single packet through the entire pipeline.
func (p *Pipeline) processPacket(raw core.RawPacket) error {
	// Step 1: Decode L2-L4
	decoded, err := p.decoder.Decode(raw)
	if err != nil {
		p.metrics.DecodeErrors.Add(1)
		return fmt.Errorf("decode failed: %w", err)
	}
	p.metrics.Decoded.Add(1)
	
	// Step 2: Parse application layer
	var parsedPayload any
	var parsedLabels core.Labels
	var payloadType string
	
	for _, parser := range p.parsers {
		if parser.CanHandle(&decoded) {
			payload, labels, err := parser.Handle(&decoded)
			if err != nil {
				p.metrics.ParseErrors.Add(1)
				slog.Debug("parser failed", "parser", parser.Name(), "error", err)
				continue
			}
			
			// Use first successful parser
			parsedPayload = payload
			parsedLabels = labels
			payloadType = parser.Name()
			p.metrics.Parsed.Add(1)
			break
		}
	}
	
	// If no parser handled the packet, use raw payload
	if parsedPayload == nil {
		parsedPayload = decoded.Payload
		payloadType = "raw"
		parsedLabels = make(core.Labels)
	}
	
	// Step 3: Build OutputPacket
	output := core.OutputPacket{
		TaskID:      p.taskID,
		AgentID:     p.agentID,
		PipelineID:  p.id,
		Timestamp:   decoded.Timestamp,
		SrcIP:       decoded.IP.SrcIP,
		DstIP:       decoded.IP.DstIP,
		SrcPort:     decoded.Transport.SrcPort,
		DstPort:     decoded.Transport.DstPort,
		Protocol:    decoded.IP.Protocol,
		Labels:      parsedLabels,
		PayloadType: payloadType,
		Payload:     parsedPayload,
		RawPayload:  decoded.Payload,
	}
	
	// Step 4: Process through processors
	for _, processor := range p.processors {
		keep := processor.Process(&output)
		p.metrics.Processed.Add(1)
		if !keep {
			// Processor dropped packet
			p.metrics.Dropped.Add(1)
			return nil
		}
	}
	
	// Step 5: Report to all reporters
	for _, reporter := range p.reporters {
		if err := reporter.Report(p.ctx, &output); err != nil {
			p.metrics.ReportErrors.Add(1)
			slog.Error("reporter failed", "reporter", reporter.Name(), "error", err)
			continue
		}
	}
	p.metrics.Reported.Add(1)
	
	return nil
}

// Stats returns pipeline statistics.
func (p *Pipeline) Stats() Stats {
	return Stats{
		Received:     p.metrics.Received.Load(),
		Decoded:      p.metrics.Decoded.Load(),
		DecodeErrors: p.metrics.DecodeErrors.Load(),
		Parsed:       p.metrics.Parsed.Load(),
		ParseErrors:  p.metrics.ParseErrors.Load(),
		Processed:    p.metrics.Processed.Load(),
		Dropped:      p.metrics.Dropped.Load(),
		Reported:     p.metrics.Reported.Load(),
		ReportErrors: p.metrics.ReportErrors.Load(),
	}
}

// Stats represents pipeline statistics.
type Stats struct {
	Received     uint64
	Decoded      uint64
	DecodeErrors uint64
	Parsed       uint64
	ParseErrors  uint64
	Processed    uint64
	Dropped      uint64
	Reported     uint64
	ReportErrors uint64
}
