// Package plugin defines the plugin lifecycle interface.
package plugin

import "context"

// Plugin is the base interface for all plugins.
type Plugin interface {
	Name() string
	Init(cfg map[string]any) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Pausable is an optional interface that plugins can implement to support
// pause/resume without full stop/start cycles.
// Useful for temporarily suspending high-load capture or processing.
type Pausable interface {
	Pause() error
	Resume() error
}

// Reconfigurable is an optional interface that plugins can implement to
// support dynamic configuration updates without restart.
// Example uses: changing Kafka topic, updating filter rules, adjusting thresholds.
type Reconfigurable interface {
	Reconfigure(cfg map[string]any) error
}

// StatsReporter is an optional interface for plugins that maintain internal
// counters.  The pipeline calls LogStats periodically and on shutdown so that
// drop/pass statistics are written to the structured log for diagnostics.
type StatsReporter interface {
	// LogStats emits a single structured log line with all counters.
	// taskID and pipelineID are supplied by the caller for correlation.
	LogStats(taskID string, pipelineID int)
}
