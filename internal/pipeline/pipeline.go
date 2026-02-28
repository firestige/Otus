// Package pipeline implements the packet processing pipeline engine.
package pipeline

import (
	"context"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"icc.tech/capture-agent/internal/core"
	"icc.tech/capture-agent/internal/core/decoder"
	"icc.tech/capture-agent/internal/metrics"
	"icc.tech/capture-agent/pkg/plugin"
)

// Pipeline represents a single-threaded packet processing chain.
// It does NOT own capture or reporter plugins - those are managed by Task.
// Pipeline receives raw packets from an input stream and outputs processed packets to an output channel.
type Pipeline struct {
	id         int
	taskID     string
	agentID    string
	decoder    decoder.Decoder
	parsers    []plugin.Parser
	processors []plugin.Processor
	metrics    *Metrics
	dropCount  atomic.Uint64 // total drops for sampled logging
}

// Config contains pipeline configuration.
type Config struct {
	ID         int
	TaskID     string
	AgentID    string
	Decoder    decoder.Decoder
	Parsers    []plugin.Parser
	Processors []plugin.Processor
}

// New creates a new pipeline.
func New(cfg Config) *Pipeline {
	return &Pipeline{
		id:         cfg.ID,
		taskID:     cfg.TaskID,
		agentID:    cfg.AgentID,
		decoder:    cfg.Decoder,
		parsers:    cfg.Parsers,
		processors: cfg.Processors,
		metrics:    NewMetrics(cfg.TaskID, cfg.ID),
	}
}

// Run starts the pipeline processing loop.
// It reads raw packets from the input stream, processes them through the decode→parse→process chain,
// and outputs the results to the output channel.
// This is the single goroutine main loop that does synchronous processing (zero internal channels).
func (p *Pipeline) Run(ctx context.Context, input <-chan core.RawPacket, output chan<- core.OutputPacket) {
	slog.Info("pipeline starting", "task_id", p.taskID, "pipeline_id", p.id)

	defer func() {
		slog.Info("pipeline stopped", "task_id", p.taskID, "pipeline_id", p.id)
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case raw, ok := <-input:
			if !ok {
				// Input stream closed
				return
			}

			p.metrics.Received.Add(1)

			// Process packet synchronously (zero channel internal passing)
			if result, ok := p.processPacket(raw); ok {
				// Non-blocking send to output
				select {
				case output <- result:
					// Sent successfully
				case <-ctx.Done():
					return
				default:
					// Output channel full, drop packet
					p.metrics.Dropped.Add(1)
					if p.dropCount.Add(1)%1000 == 1 {
						slog.Warn("pipeline output full, dropping packets",
							"task_id", p.taskID, "pipeline_id", p.id,
							"total_dropped", p.dropCount.Load())
					}
				}
			}
		}
	}
}

// processPacket processes a single packet through the entire pipeline.
// Returns the output packet and a boolean indicating whether to forward it.
func (p *Pipeline) processPacket(raw core.RawPacket) (core.OutputPacket, bool) {
	startTime := time.Now()

	// Step 1: Decode L2-L4
	pipelineID := strconv.Itoa(p.id)

	decoded, err := p.decoder.Decode(raw)
	if err != nil {
		p.metrics.DecodeErrors.Add(1)
		metrics.PipelinePacketsTotal.WithLabelValues(p.taskID, pipelineID, "decode_error").Inc()
		return core.OutputPacket{}, false
	}
	p.metrics.Decoded.Add(1)
	metrics.PipelinePacketsTotal.WithLabelValues(p.taskID, pipelineID, "decoded").Inc()

	// Measure decode latency
	decodeLatency := time.Since(startTime).Seconds()
	metrics.PipelineLatencySeconds.WithLabelValues(p.taskID, "decode").Observe(decodeLatency)

	// Step 2: Parse application layer
	parseStart := time.Now()
	var parsedPayload any
	var parsedLabels core.Labels
	var payloadType string
	var parserMatched bool

	for _, parser := range p.parsers {
		if parser.CanHandle(&decoded) {
			payload, labels, err := parser.Handle(&decoded)
			if err != nil {
				p.metrics.ParseErrors.Add(1)
				metrics.PipelinePacketsTotal.WithLabelValues(p.taskID, pipelineID, "parse_error").Inc()
				slog.Debug("parser failed", "parser", parser.Name(), "error", err)
				continue
			}

			// Use first successful parser.
			// Note: parsedPayload may be nil (e.g. SIP parser returns nil — raw bytes are
			// preserved in OutputPacket.RawPayload). parserMatched tracks whether a parser
			// succeeded regardless of the payload value.
			parsedPayload = payload
			parsedLabels = labels
			payloadType = parser.Name()
			parserMatched = true
			p.metrics.Parsed.Add(1)
			metrics.PipelinePacketsTotal.WithLabelValues(p.taskID, pipelineID, "parsed").Inc()
			break
		}
	}

	// Measure parse latency
	if parserMatched {
		parseLatency := time.Since(parseStart).Seconds()
		metrics.PipelineLatencySeconds.WithLabelValues(p.taskID, "parse").Observe(parseLatency)
	}

	// If no parser handled the packet, fall back to raw payload type.
	// This is distinct from a parser that ran but returned a nil typed payload
	// (e.g. SIP, which stores everything in Labels + OutputPacket.RawPayload).
	if !parserMatched {
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
	processStart := time.Now()
	for _, processor := range p.processors {
		keep := processor.Process(&output)
		p.metrics.Processed.Add(1)
		if !keep {
			// Processor dropped packet
			p.metrics.Dropped.Add(1)
			metrics.PipelinePacketsTotal.WithLabelValues(p.taskID, pipelineID, "dropped").Inc()
			return core.OutputPacket{}, false
		}
	}

	// Measure processor latency
	if len(p.processors) > 0 {
		processLatency := time.Since(processStart).Seconds()
		metrics.PipelineLatencySeconds.WithLabelValues(p.taskID, "process").Observe(processLatency)
	}

	// Measure full pipeline end-to-end latency
	totalLatency := time.Since(startTime).Seconds()
	metrics.PipelineLatencySeconds.WithLabelValues(p.taskID, "total").Observe(totalLatency)

	metrics.PipelinePacketsTotal.WithLabelValues(p.taskID, pipelineID, "output").Inc()

	return output, true
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
	}
}

// Parsers returns the pipeline's parser instances for lifecycle management.
func (p *Pipeline) Parsers() []plugin.Parser {
	return p.parsers
}

// Processors returns the pipeline's processor instances for lifecycle management.
func (p *Pipeline) Processors() []plugin.Processor {
	return p.processors
}

// Stats represents pipeline statistics.
// Reporter statistics (Reported, ReportErrors) are tracked at Task level.
type Stats struct {
	Received     uint64
	Decoded      uint64
	DecodeErrors uint64
	Parsed       uint64
	ParseErrors  uint64
	Processed    uint64
	Dropped      uint64
}
