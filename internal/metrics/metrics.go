// Package metrics implements Prometheus metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// CapturePacketsTotal counts total packets captured by interface
	CapturePacketsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "capture_agent_capture_packets_total",
			Help: "Total number of packets captured",
		},
		[]string{"task", "interface"},
	)

	// CaptureDropsTotal counts total packets dropped during capture
	CaptureDropsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "capture_agent_capture_drops_total",
			Help: "Total number of packets dropped during capture",
		},
		[]string{"task", "stage"},
	)

	// PipelinePacketsTotal counts total packets processed in pipeline
	PipelinePacketsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "capture_agent_pipeline_packets_total",
			Help: "Total number of packets processed in pipeline",
		},
		[]string{"task", "pipeline", "stage"},
	)

	// PipelineLatencySeconds measures pipeline stage latency
	PipelineLatencySeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "capture_agent_pipeline_latency_seconds",
			Help:    "Latency of pipeline processing stages in seconds",
			Buckets: prometheus.ExponentialBuckets(0.000001, 2, 20), // 1µs to ~1s
		},
		[]string{"task", "stage"},
	)

	// TaskStatus tracks current task status
	TaskStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capture_agent_task_status",
			Help: "Current status of tasks (0=stopped, 1=running, 2=error)",
		},
		[]string{"task", "status"},
	)

	// ReassemblyActiveFragments tracks active IP fragments awaiting reassembly
	ReassemblyActiveFragments = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "capture_agent_reassembly_active_fragments",
			Help: "Number of active IP fragments in reassembly queue",
		},
	)

	// ReporterBatchSize tracks Kafka batch size distribution (for ReporterWrapper)
	ReporterBatchSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "capture_agent_reporter_batch_size",
			Help:    "Number of packets sent per reporter batch",
			Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1, 2, 4, ..., 2048
		},
		[]string{"task", "reporter"},
	)

	// ReporterErrorsTotal counts reporter errors by name and error type
	ReporterErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "capture_agent_reporter_errors_total",
			Help: "Total number of reporter errors",
		},
		[]string{"task", "reporter", "error_type"},
	)

	// FlowRegistrySize tracks the current number of flows in a task's FlowRegistry
	FlowRegistrySize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "capture_agent_flow_registry_size",
			Help: "Current number of flows tracked in the flow registry",
		},
		[]string{"task"},
	)
)

// TaskStatusValue represents task status as a numeric value for Prometheus gauge
const (
	TaskStatusStopped = 0
	TaskStatusRunning = 1
	TaskStatusError   = 2
	TaskStatusPaused  = 3
)
