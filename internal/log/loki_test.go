package log

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewLokiWriter(t *testing.T) {
	cfg := LokiConfig{
		Endpoint:      "http://localhost:3100/loki/api/v1/push",
		Labels:        map[string]string{"service": "test"},
		BatchSize:     10,
		FlushInterval: "1s",
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}
	defer lw.Close()

	if lw.endpoint != cfg.Endpoint {
		t.Errorf("Expected endpoint %s, got %s", cfg.Endpoint, lw.endpoint)
	}
	if lw.batchSize != cfg.BatchSize {
		t.Errorf("Expected batch size %d, got %d", cfg.BatchSize, lw.batchSize)
	}
	if lw.flushInterval != time.Second {
		t.Errorf("Expected flush interval 1s, got %v", lw.flushInterval)
	}
}

func TestNewLokiWriterDefaultBatchSize(t *testing.T) {
	cfg := LokiConfig{
		Endpoint:  "http://localhost:3100/loki/api/v1/push",
		BatchSize: 0, // Should default to 100
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}
	defer lw.Close()

	if lw.batchSize != 100 {
		t.Errorf("Expected default batch size 100, got %d", lw.batchSize)
	}
}

func TestNewLokiWriterDefaultLabels(t *testing.T) {
	cfg := LokiConfig{
		Endpoint: "http://localhost:3100/loki/api/v1/push",
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}
	defer lw.Close()

	if lw.labels["job"] != "otus" {
		t.Errorf("Expected default job label 'otus', got %s", lw.labels["job"])
	}
}

func TestNewLokiWriterInvalidFlushInterval(t *testing.T) {
	cfg := LokiConfig{
		Endpoint:      "http://localhost:3100/loki/api/v1/push",
		FlushInterval: "invalid",
	}

	_, err := NewLokiWriter(cfg)
	if err == nil {
		t.Error("Expected error for invalid flush interval, got nil")
	}
}

func TestLokiWriterWrite(t *testing.T) {
	cfg := LokiConfig{
		Endpoint:  "http://localhost:3100/loki/api/v1/push",
		BatchSize: 10,
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}
	defer lw.Close()

	// Write a log entry
	n, err := lw.Write([]byte("test log message"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 16 {
		t.Errorf("Expected 16 bytes written, got %d", n)
	}

	// Verify batch contains the entry
	lw.mu.Lock()
	batchLen := len(lw.batch)
	lw.mu.Unlock()

	if batchLen != 1 {
		t.Errorf("Expected 1 entry in batch, got %d", batchLen)
	}
}

func TestLokiWriterWriteAfterClose(t *testing.T) {
	cfg := LokiConfig{
		Endpoint:  "http://localhost:3100/loki/api/v1/push",
		BatchSize: 10,
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}

	// Close the writer
	lw.Close()

	// Try to write after close
	_, err = lw.Write([]byte("test"))
	if err == nil {
		t.Error("Expected error when writing after close, got nil")
	}
}

func TestLokiWriterBatchFlush(t *testing.T) {
	var requestCount atomic.Int32

	// Create mock Loki server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)

		// Verify request method and path
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Read and parse request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var pushReq lokiPushRequest
		if err := json.Unmarshal(body, &pushReq); err != nil {
			t.Errorf("Failed to parse request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Verify request structure
		if len(pushReq.Streams) != 1 {
			t.Errorf("Expected 1 stream, got %d", len(pushReq.Streams))
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := LokiConfig{
		Endpoint:  server.URL,
		BatchSize: 3, // Small batch for testing
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}
	defer lw.Close()

	// Write logs to fill the batch
	for i := 0; i < 3; i++ {
		_, err := lw.Write([]byte(fmt.Sprintf("log message %d\n", i)))
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
	}

	// Wait a bit for async flush
	time.Sleep(100 * time.Millisecond)

	// Verify at least one request was sent
	if requestCount.Load() < 1 {
		t.Errorf("Expected at least 1 request, got %d", requestCount.Load())
	}
}

func TestLokiWriterPeriodicFlush(t *testing.T) {
	var requestCount atomic.Int32

	// Create mock Loki server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := LokiConfig{
		Endpoint:      server.URL,
		BatchSize:     100, // Large batch to prevent immediate flush
		FlushInterval: "100ms",
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}
	defer lw.Close()

	// Write a single log entry
	_, err = lw.Write([]byte("test log\n"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Wait for periodic flush
	time.Sleep(200 * time.Millisecond)

	// Verify periodic flush happened
	if requestCount.Load() < 1 {
		t.Errorf("Expected at least 1 periodic flush, got %d requests", requestCount.Load())
	}
}

func TestLokiWriterCloseFlush(t *testing.T) {
	var requestCount atomic.Int32

	// Create mock Loki server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := LokiConfig{
		Endpoint:      server.URL,
		BatchSize:     100,   // Large batch
		FlushInterval: "10s", // Long interval
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}

	// Write logs but don't reach batch size
	for i := 0; i < 5; i++ {
		_, err := lw.Write([]byte(fmt.Sprintf("log %d\n", i)))
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
	}

	// Close should flush remaining logs
	lw.Close()

	// Verify flush on close
	if requestCount.Load() != 1 {
		t.Errorf("Expected 1 request on close, got %d", requestCount.Load())
	}
}

func TestLokiWriterRetry(t *testing.T) {
	var attemptCount atomic.Int32
	maxAttempts := int32(2)

	// Create mock server that fails first, then succeeds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attemptCount.Add(1)
		if attempt < maxAttempts {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	cfg := LokiConfig{
		Endpoint:  server.URL,
		BatchSize: 1, // Immediate flush
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}
	defer lw.Close()

	// Write log (should trigger flush with retry)
	_, err = lw.Write([]byte("test log\n"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Wait for retries
	time.Sleep(500 * time.Millisecond)

	// Verify retry happened
	if attemptCount.Load() < 2 {
		t.Errorf("Expected at least 2 attempts, got %d", attemptCount.Load())
	}
}

func TestLokiWriterHTTPError(t *testing.T) {
	// Create mock server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	cfg := LokiConfig{
		Endpoint:  server.URL,
		BatchSize: 1, // Immediate flush
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}
	defer lw.Close()

	// Write log (flush will fail but shouldn't return error from Write)
	_, err = lw.Write([]byte("test log\n"))
	if err != nil {
		t.Errorf("Write should not fail even if flush fails: %v", err)
	}

	// Give time for flush attempts
	time.Sleep(500 * time.Millisecond)
}

func TestLokiPushRequestFormat(t *testing.T) {
	var receivedBody []byte

	// Create mock server to capture request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := LokiConfig{
		Endpoint:  server.URL,
		Labels:    map[string]string{"service": "test", "env": "dev"},
		BatchSize: 1,
	}

	lw, err := NewLokiWriter(cfg)
	if err != nil {
		t.Fatalf("NewLokiWriter failed: %v", err)
	}
	defer lw.Close()

	// Write log
	logMsg := "test log message\n"
	lw.Write([]byte(logMsg))

	// Wait for flush
	time.Sleep(100 * time.Millisecond)

	// Parse received body
	var pushReq lokiPushRequest
	if err := json.Unmarshal(receivedBody, &pushReq); err != nil {
		t.Fatalf("Failed to parse request body: %v", err)
	}

	// Verify structure
	if len(pushReq.Streams) != 1 {
		t.Fatalf("Expected 1 stream, got %d", len(pushReq.Streams))
	}

	stream := pushReq.Streams[0]

	// Verify labels
	if stream.Stream["service"] != "test" {
		t.Errorf("Expected service label 'test', got %s", stream.Stream["service"])
	}
	if stream.Stream["env"] != "dev" {
		t.Errorf("Expected env label 'dev', got %s", stream.Stream["env"])
	}

	// Verify values
	if len(stream.Values) != 1 {
		t.Fatalf("Expected 1 value, got %d", len(stream.Values))
	}
	if len(stream.Values[0]) != 2 {
		t.Fatalf("Expected [timestamp, line], got %v", stream.Values[0])
	}

	// Verify log line
	if !strings.Contains(stream.Values[0][1], logMsg) {
		t.Errorf("Expected log message %q in %q", logMsg, stream.Values[0][1])
	}
}
