// Package task implements task lifecycle management.
package task

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"icc.tech/capture-agent/internal/core"
	"icc.tech/capture-agent/internal/metrics"
	"icc.tech/capture-agent/pkg/plugin"
)

const (
	defaultWrapperBatchSize    = 100
	defaultWrapperBatchTimeout = 50 * time.Millisecond
	defaultWrapperChanCap      = 10000
)

// ReporterWrapper wraps a Reporter with batching and optional fallback.
// It sits between senderLoop and the actual Reporter plugin:
//
//	senderLoop → ReporterWrapper.Send() → batchLoop → Reporter.ReportBatch()/Report()
//	                                                 └→ fallback Reporter (on primary failure)
type ReporterWrapper struct {
	primary  plugin.Reporter
	fallback plugin.Reporter // nil if no fallback configured

	taskID       string // for Prometheus label
	batchSize    int
	batchTimeout time.Duration

	batchCh   chan *core.OutputPacket
	doneCh    chan struct{}
	dropCount atomic.Uint64 // cumulative silent drops (batchCh full)
}

// WrapperConfig contains configuration for creating a ReporterWrapper.
type WrapperConfig struct {
	Primary      plugin.Reporter
	Fallback     plugin.Reporter // nil if no fallback
	TaskID       string          // task ID for Prometheus labels
	BatchSize    int
	BatchTimeout time.Duration
	NoBatch      bool // if true, each packet is flushed immediately (batchSize forced to 1)
}

// NewReporterWrapper creates a new wrapper around a Reporter.
func NewReporterWrapper(cfg WrapperConfig) *ReporterWrapper {
	batchSize := cfg.BatchSize
	if cfg.NoBatch {
		// Force batchSize=1 so every packet is flushed immediately by batchLoop.
		batchSize = 1
	} else if batchSize <= 0 {
		batchSize = defaultWrapperBatchSize
	}
	batchTimeout := cfg.BatchTimeout
	if batchTimeout <= 0 {
		batchTimeout = defaultWrapperBatchTimeout
	}

	return &ReporterWrapper{
		primary:      cfg.Primary,
		fallback:     cfg.Fallback,
		taskID:       cfg.TaskID,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		batchCh:      make(chan *core.OutputPacket, defaultWrapperChanCap),
		doneCh:       make(chan struct{}),
	}
}

// Start starts the batchLoop goroutine. Does NOT start the underlying reporters
// (those are started separately by Task.Start).
func (w *ReporterWrapper) Start(ctx context.Context) {
	go w.batchLoop(ctx)
}

// Send enqueues a packet for batched delivery. Non-blocking: if the channel
// is full the packet is dropped and counted in ReporterDropsTotal.
func (w *ReporterWrapper) Send(pkt *core.OutputPacket) {
	select {
	case w.batchCh <- pkt:
	default:
		w.dropCount.Add(1)
		metrics.ReporterDropsTotal.WithLabelValues(w.taskID, w.primary.Name()).Inc()
	}
}

// DropsTotal returns the cumulative number of packets silently dropped because
// batchCh was full at the time of Send().
func (w *ReporterWrapper) DropsTotal() uint64 {
	return w.dropCount.Load()
}

// Close closes the batch channel and waits for all pending packets to flush.
func (w *ReporterWrapper) Close() {
	close(w.batchCh)
	<-w.doneCh
}

// batchLoop collects packets into batches and flushes on size or timeout.
func (w *ReporterWrapper) batchLoop(ctx context.Context) {
	defer close(w.doneCh)

	batch := make([]*core.OutputPacket, 0, w.batchSize)
	ticker := time.NewTicker(w.batchTimeout)
	defer ticker.Stop()

	var flushCount uint64

	flush := func() {
		if len(batch) == 0 {
			return
		}

		// Capture queue depth before flushing — shows how backed-up the channel is.
		queueDepth := len(w.batchCh)
		flushCount++

		start := time.Now()
		if err := w.sendBatch(ctx, batch); err != nil {
			slog.Warn("primary reporter batch failed",
				"reporter", w.primary.Name(),
				"batch_size", len(batch),
				"error", err)
			// Fallback: send each packet to fallback reporter
			if w.fallback != nil {
				for _, pkt := range batch {
					if fbErr := w.fallback.Report(ctx, pkt); fbErr != nil {
						metrics.ReporterErrorsTotal.WithLabelValues(w.taskID, w.fallback.Name(), "fallback").Inc()
						slog.Warn("fallback reporter also failed",
							"reporter", w.fallback.Name(),
							"error", fbErr)
					}
				}
			}
		}
		elapsed := time.Since(start)

		// Warn if a single flush takes suspiciously long (suggests reporter blocking).
		if elapsed > 5*time.Millisecond {
			slog.Warn("reporter flush slow — reporter may be blocking",
				"reporter", w.primary.Name(),
				"task_id", w.taskID,
				"batch_size", len(batch),
				"duration_ms", elapsed.Milliseconds(),
				"batchCh_queue", queueDepth,
				"batchCh_cap", cap(w.batchCh),
			)
		} else if flushCount%500 == 0 {
			// Periodic heartbeat: shows steady-state queue depth and send latency.
			slog.Debug("reporter flush stats",
				"reporter", w.primary.Name(),
				"task_id", w.taskID,
				"flush_count", flushCount,
				"batchCh_queue", queueDepth,
				"batchCh_cap", cap(w.batchCh),
				"last_flush_us", elapsed.Microseconds(),
			)
		}

		batch = batch[:0]
	}

	for {
		select {
		case pkt, ok := <-w.batchCh:
			if !ok {
				// Channel closed — flush remaining and exit
				flush()
				return
			}
			batch = append(batch, pkt)
			if len(batch) >= w.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// sendBatch sends a batch of packets using BatchReporter if available,
// otherwise falls back to calling Report() one-by-one.
func (w *ReporterWrapper) sendBatch(ctx context.Context, batch []*core.OutputPacket) error {
	reporterName := w.primary.Name()

	// Record batch size metric
	metrics.ReporterBatchSize.WithLabelValues(w.taskID, reporterName).
		Observe(float64(len(batch)))

	// Prefer BatchReporter interface for high-throughput reporters (e.g., Kafka)
	if br, ok := w.primary.(plugin.BatchReporter); ok {
		if err := br.ReportBatch(ctx, batch); err != nil {
			metrics.ReporterErrorsTotal.WithLabelValues(w.taskID, reporterName, "batch").Inc()
			return err
		}
		return nil
	}

	// Fallback: sequential Report() calls
	var lastErr error
	for _, pkt := range batch {
		if err := w.primary.Report(ctx, pkt); err != nil {
			metrics.ReporterErrorsTotal.WithLabelValues(w.taskID, reporterName, "report").Inc()
			lastErr = err
		}
	}
	return lastErr
}
