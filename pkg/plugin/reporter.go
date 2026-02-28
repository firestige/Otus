// Package plugin defines plugin interfaces.
package plugin

import (
	"context"

	"icc.tech/capture-agent/internal/core"
)

// Reporter sends output packets to external systems.
type Reporter interface {
	Plugin
	Report(ctx context.Context, pkt *core.OutputPacket) error
	Flush(ctx context.Context) error
}

// BatchReporter is an optional interface that Reporter plugins can implement
// to receive packets in batches for higher throughput (e.g., Kafka batch writes).
// Reporters that don't implement this interface will have packets sent one-by-one
// via Report() by the ReporterWrapper.
type BatchReporter interface {
	Reporter
	ReportBatch(ctx context.Context, pkts []*core.OutputPacket) error
}
