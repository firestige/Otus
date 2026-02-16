// Package pipeline implements pipeline construction.
package pipeline

import (
	"firestige.xyz/otus/internal/core/decoder"
	"firestige.xyz/otus/pkg/plugin"
)

// Builder provides a fluent interface for building pipelines.
// This is an alternative to using Config directly.
type Builder struct {
	config Config
}

// NewBuilder creates a new pipeline builder.
func NewBuilder() *Builder {
	return &Builder{
		config: Config{
			BufferSize: 1024, // default
		},
	}
}

// WithID sets the pipeline ID.
func (b *Builder) WithID(id int) *Builder {
	b.config.ID = id
	return b
}

// WithTaskID sets the task ID.
func (b *Builder) WithTaskID(taskID string) *Builder {
	b.config.TaskID = taskID
	return b
}

// WithAgentID sets the agent ID.
func (b *Builder) WithAgentID(agentID string) *Builder {
	b.config.AgentID = agentID
	return b
}

// WithCapturer sets the packet capturer.
func (b *Builder) WithCapturer(c plugin.Capturer) *Builder {
	b.config.Capturer = c
	return b
}

// WithDecoder sets the packet decoder.
func (b *Builder) WithDecoder(d decoder.Decoder) *Builder {
	b.config.Decoder = d
	return b
}

// WithParsers sets the parser chain.
func (b *Builder) WithParsers(parsers ...plugin.Parser) *Builder {
	b.config.Parsers = parsers
	return b
}

// WithProcessors sets the processor chain.
func (b *Builder) WithProcessors(processors ...plugin.Processor) *Builder {
	b.config.Processors = processors
	return b
}

// WithReporters sets the reporter chain.
func (b *Builder) WithReporters(reporters ...plugin.Reporter) *Builder {
	b.config.Reporters = reporters
	return b
}

// WithBufferSize sets the raw packet channel buffer size.
func (b *Builder) WithBufferSize(size int) *Builder {
	b.config.BufferSize = size
	return b
}

// Build creates the pipeline.
func (b *Builder) Build() *Pipeline {
	return New(b.config)
}

