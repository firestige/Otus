// Package log implements log outputs.
package log

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// LokiConfig contains configuration for Loki writer.
type LokiConfig struct {
	Endpoint      string            // Loki push endpoint URL
	Labels        map[string]string // Stream labels
	BatchSize     int               // Number of log entries per batch
	FlushInterval string            // Flush interval (e.g., "5s")
}

// LokiWriter implements io.Writer and sends logs to Grafana Loki.
type LokiWriter struct {
	endpoint      string
	labels        map[string]string
	batchSize     int
	flushInterval time.Duration
	httpClient    *http.Client

	mu      sync.Mutex
	batch   []logEntry
	closed  bool
	closeCh chan struct{}
	wg      sync.WaitGroup
}

// logEntry represents a single log entry with timestamp.
type logEntry struct {
	timestamp time.Time
	line      string
}

// lokiPushRequest represents the Loki push API request format.
type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

// lokiStream represents a single stream in Loki push request.
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// NewLokiWriter creates a new Loki writer.
func NewLokiWriter(cfg LokiConfig) (*LokiWriter, error) {
	// Parse flush interval
	flushInterval := 5 * time.Second // default
	if cfg.FlushInterval != "" {
		duration, err := time.ParseDuration(cfg.FlushInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid flush interval: %w", err)
		}
		flushInterval = duration
	}

	// Default batch size
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	// Default labels
	labels := cfg.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	if _, ok := labels["job"]; !ok {
		labels["job"] = "otus"
	}

	lw := &LokiWriter{
		endpoint:      cfg.Endpoint,
		labels:        labels,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		batch:   make([]logEntry, 0, batchSize),
		closeCh: make(chan struct{}),
	}

	// Start background flusher
	lw.wg.Add(1)
	go lw.flusher()

	return lw, nil
}

// Write implements io.Writer interface.
func (lw *LokiWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.closed {
		return 0, fmt.Errorf("loki writer is closed")
	}

	// Add log entry to batch
	entry := logEntry{
		timestamp: time.Now(),
		line:      string(p),
	}
	lw.batch = append(lw.batch, entry)

	// Flush if batch is full
	if len(lw.batch) >= lw.batchSize {
		if err := lw.flushLocked(); err != nil {
			// Log error but don't fail the write
			fmt.Fprintf(io.Discard, "loki flush error: %v\n", err)
		}
	}

	return len(p), nil
}

// Close closes the writer and flushes remaining logs.
func (lw *LokiWriter) Close() error {
	lw.mu.Lock()
	if lw.closed {
		lw.mu.Unlock()
		return nil
	}
	lw.closed = true

	// Flush remaining logs
	err := lw.flushLocked()
	lw.mu.Unlock()

	// Stop background flusher
	close(lw.closeCh)
	lw.wg.Wait()

	return err
}

// flusher runs in background and flushes logs periodically.
func (lw *LokiWriter) flusher() {
	defer lw.wg.Done()

	ticker := time.NewTicker(lw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lw.mu.Lock()
			if !lw.closed && len(lw.batch) > 0 {
				if err := lw.flushLocked(); err != nil {
					fmt.Fprintf(io.Discard, "loki background flush error: %v\n", err)
				}
			}
			lw.mu.Unlock()

		case <-lw.closeCh:
			return
		}
	}
}

// flushLocked sends batched logs to Loki.
// Must be called with lw.mu locked.
func (lw *LokiWriter) flushLocked() error {
	if len(lw.batch) == 0 {
		return nil
	}

	// Build push request
	values := make([][]string, len(lw.batch))
	for i, entry := range lw.batch {
		values[i] = []string{
			fmt.Sprintf("%d", entry.timestamp.UnixNano()),
			entry.line,
		}
	}

	req := lokiPushRequest{
		Streams: []lokiStream{
			{
				Stream: lw.labels,
				Values: values,
			},
		},
	}

	// Marshal request
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal loki request: %w", err)
	}

	// Send HTTP request with retry
	err = lw.sendWithRetry(data)
	if err != nil {
		return err
	}

	// Clear batch
	lw.batch = lw.batch[:0]
	return nil
}

// sendWithRetry sends HTTP request to Loki with exponential backoff retry.
func (lw *LokiWriter) sendWithRetry(data []byte) error {
	maxRetries := 3
	baseDelay := 100 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			time.Sleep(delay)
		}

		err := lw.send(data)
		if err == nil {
			return nil
		}

		lastErr = err
	}

	return fmt.Errorf("loki push failed after %d retries: %w", maxRetries, lastErr)
}

// send sends a single HTTP request to Loki.
func (lw *LokiWriter) send(data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", lw.endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := lw.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("loki push failed with status %d: %s", resp.StatusCode, body)
	}

	return nil
}
