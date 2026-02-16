package pipeline

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/pkg/plugin"
)

// Mock implementations for testing

// MockCapturer is a mock packet capturer.
type MockCapturer struct {
	name    string
	packets []core.RawPacket
	delay   time.Duration
	mu      sync.Mutex
}

func NewMockCapturer(name string, packets []core.RawPacket) *MockCapturer {
	return &MockCapturer{
		name:    name,
		packets: packets,
	}
}

func (m *MockCapturer) Name() string { return m.name }

func (m *MockCapturer) Init(config map[string]any) error { return nil }

func (m *MockCapturer) Start(ctx context.Context) error { return nil }

func (m *MockCapturer) Stop(ctx context.Context) error { return nil }

func (m *MockCapturer) Capture(ctx context.Context, out chan<- core.RawPacket) error {
	for _, pkt := range m.packets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- pkt:
			if m.delay > 0 {
				time.Sleep(m.delay)
			}
		}
	}
	// Keep running until context is cancelled
	<-ctx.Done()
	return ctx.Err()
}

func (m *MockCapturer) Stats() plugin.CaptureStats {
	return plugin.CaptureStats{
		PacketsReceived: uint64(len(m.packets)),
		PacketsDropped:  0,
	}
}

// MockDecoder is a mock decoder.
type MockDecoder struct {
	shouldFail bool
}

func NewMockDecoder() *MockDecoder {
	return &MockDecoder{}
}

func (m *MockDecoder) Decode(raw core.RawPacket) (core.DecodedPacket, error) {
	if m.shouldFail {
		return core.DecodedPacket{}, core.ErrPacketTooShort
	}

	// Create a simple decoded packet
	srcIP := netip.MustParseAddr("192.168.1.1")
	dstIP := netip.MustParseAddr("192.168.1.2")

	return core.DecodedPacket{
		Timestamp: raw.Timestamp,
		Ethernet: core.EthernetHeader{
			EtherType: 0x0800, // IPv4
		},
		IP: core.IPHeader{
			Version:  4,
			SrcIP:    srcIP,
			DstIP:    dstIP,
			Protocol: 17, // UDP
		},
		Transport: core.TransportHeader{
			SrcPort:  5060,
			DstPort:  5060,
			Protocol: 17, // UDP
		},
		Payload:    raw.Data,
		CaptureLen: raw.CaptureLen,
		OrigLen:    raw.OrigLen,
	}, nil
}

// MockParser is a mock parser.
type MockParser struct {
	name       string
	canHandle  bool
	shouldFail bool
	mu         sync.Mutex
	handled    []core.DecodedPacket
}

func NewMockParser(name string, canHandle bool) *MockParser {
	return &MockParser{
		name:      name,
		canHandle: canHandle,
		handled:   make([]core.DecodedPacket, 0),
	}
}

func (m *MockParser) Name() string { return m.name }

func (m *MockParser) Init(config map[string]any) error { return nil }

func (m *MockParser) Start(ctx context.Context) error { return nil }

func (m *MockParser) Stop(ctx context.Context) error { return nil }

func (m *MockParser) CanHandle(decoded *core.DecodedPacket) bool {
	return m.canHandle
}

func (m *MockParser) Handle(decoded *core.DecodedPacket) (any, core.Labels, error) {
	m.mu.Lock()
	m.handled = append(m.handled, *decoded)
	m.mu.Unlock()

	if m.shouldFail {
		return nil, nil, core.ErrConfigInvalid // Use some sentinel error
	}

	return map[string]string{"parsed": "data"}, core.Labels{"protocol": "SIP"}, nil
}

func (m *MockParser) HandledCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.handled)
}

// MockProcessor is a mock processor.
type MockProcessor struct {
	name       string
	shouldDrop bool
	mu         sync.Mutex
	processed  []core.OutputPacket
}

func NewMockProcessor(name string, shouldDrop bool) *MockProcessor {
	return &MockProcessor{
		name:       name,
		shouldDrop: shouldDrop,
		processed:  make([]core.OutputPacket, 0),
	}
}

func (m *MockProcessor) Name() string { return m.name }

func (m *MockProcessor) Init(config map[string]any) error { return nil }

func (m *MockProcessor) Start(ctx context.Context) error { return nil }

func (m *MockProcessor) Stop(ctx context.Context) error { return nil }

func (m *MockProcessor) Process(pkt *core.OutputPacket) bool {
	m.mu.Lock()
	m.processed = append(m.processed, *pkt)
	m.mu.Unlock()

	return !m.shouldDrop
}

func (m *MockProcessor) ProcessedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.processed)
}

// MockReporter is a mock reporter.
type MockReporter struct {
	name     string
	mu       sync.Mutex
	reported []core.OutputPacket
}

func NewMockReporter(name string) *MockReporter {
	return &MockReporter{
		name:     name,
		reported: make([]core.OutputPacket, 0),
	}
}

func (m *MockReporter) Name() string { return m.name }

func (m *MockReporter) Init(config map[string]any) error { return nil }

func (m *MockReporter) Start(ctx context.Context) error { return nil }

func (m *MockReporter) Stop(ctx context.Context) error { return nil }

func (m *MockReporter) Report(ctx context.Context, pkt *core.OutputPacket) error {
	m.mu.Lock()
	m.reported = append(m.reported, *pkt)
	m.mu.Unlock()
	return nil
}

func (m *MockReporter) Flush(ctx context.Context) error {
	return nil
}

func (m *MockReporter) ReportedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.reported)
}

// Test cases

func TestPipeline_BasicFlow(t *testing.T) {
	// Create test packets
	packets := []core.RawPacket{
		{
			Timestamp:  time.Now(),
			Data:       []byte("packet1"),
			CaptureLen: 7,
			OrigLen:    7,
		},
		{
			Timestamp:  time.Now(),
			Data:       []byte("packet2"),
			CaptureLen: 7,
			OrigLen:    7,
		},
	}

	// Create mock components
	capturer := NewMockCapturer("mock-capturer", packets)
	decoder := NewMockDecoder()
	parser := NewMockParser("mock-parser", true)
	processor := NewMockProcessor("mock-processor", false)
	reporter := NewMockReporter("mock-reporter")

	// Build pipeline
	pipeline := New(Config{
		ID:         1,
		TaskID:     "test-task",
		AgentID:    "test-agent",
		Capturer:   capturer,
		Decoder:    decoder,
		Parsers:    []plugin.Parser{parser},
		Processors: []plugin.Processor{processor},
		Reporters:  []plugin.Reporter{reporter},
		BufferSize: 10,
	})

	// Start pipeline
	if err := pipeline.Start(); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Stop pipeline
	if err := pipeline.Stop(); err != nil {
		t.Fatalf("Failed to stop pipeline: %v", err)
	}

	// Verify metrics
	stats := pipeline.Stats()
	if stats.Received != 2 {
		t.Errorf("Expected 2 received packets, got %d", stats.Received)
	}
	if stats.Decoded != 2 {
		t.Errorf("Expected 2 decoded packets, got %d", stats.Decoded)
	}
	if stats.Parsed != 2 {
		t.Errorf("Expected 2 parsed packets, got %d", stats.Parsed)
	}
	if stats.Processed != 2 {
		t.Errorf("Expected 2 processed packets, got %d", stats.Processed)
	}
	if stats.Reported != 2 {
		t.Errorf("Expected 2 reported packets, got %d", stats.Reported)
	}

	// Verify mock components
	if parser.HandledCount() != 2 {
		t.Errorf("Expected parser to handle 2 packets, got %d", parser.HandledCount())
	}
	if processor.ProcessedCount() != 2 {
		t.Errorf("Expected processor to process 2 packets, got %d", processor.ProcessedCount())
	}
	if reporter.ReportedCount() != 2 {
		t.Errorf("Expected reporter to report 2 packets, got %d", reporter.ReportedCount())
	}
}

func TestPipeline_ProcessorDrop(t *testing.T) {
	// Create test packets
	packets := []core.RawPacket{
		{
			Timestamp:  time.Now(),
			Data:       []byte("packet1"),
			CaptureLen: 7,
			OrigLen:    7,
		},
	}

	// Create mock components with dropping processor
	capturer := NewMockCapturer("mock-capturer", packets)
	decoder := NewMockDecoder()
	parser := NewMockParser("mock-parser", true)
	processor := NewMockProcessor("mock-processor", true) // drops all
	reporter := NewMockReporter("mock-reporter")

	// Build pipeline
	pipeline := New(Config{
		ID:         2,
		TaskID:     "test-task",
		AgentID:    "test-agent",
		Capturer:   capturer,
		Decoder:    decoder,
		Parsers:    []plugin.Parser{parser},
		Processors: []plugin.Processor{processor},
		Reporters:  []plugin.Reporter{reporter},
		BufferSize: 10,
	})

	// Start pipeline
	if err := pipeline.Start(); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Stop pipeline
	if err := pipeline.Stop(); err != nil {
		t.Fatalf("Failed to stop pipeline: %v", err)
	}

	// Verify metrics
	stats := pipeline.Stats()
	if stats.Processed != 1 {
		t.Errorf("Expected 1 processed packet, got %d", stats.Processed)
	}
	if stats.Dropped != 1 {
		t.Errorf("Expected 1 dropped packet, got %d", stats.Dropped)
	}
	if stats.Reported != 0 {
		t.Errorf("Expected 0 reported packets, got %d", stats.Reported)
	}

	// Verify reporter got nothing
	if reporter.ReportedCount() != 0 {
		t.Errorf("Expected reporter to report 0 packets, got %d", reporter.ReportedCount())
	}
}

func TestBuilder_FluentAPI(t *testing.T) {
	// Create test packets
	packets := []core.RawPacket{
		{
			Timestamp:  time.Now(),
			Data:       []byte("packet1"),
			CaptureLen: 7,
			OrigLen:    7,
		},
	}

	// Build using fluent API
	pipeline := NewBuilder().
		WithID(3).
		WithTaskID("test-task").
		WithAgentID("test-agent").
		WithCapturer(NewMockCapturer("capturer", packets)).
		WithDecoder(NewMockDecoder()).
		WithParsers(NewMockParser("parser", true)).
		WithProcessors(NewMockProcessor("processor", false)).
		WithReporters(NewMockReporter("reporter")).
		WithBufferSize(100).
		Build()

	// Start and stop
	if err := pipeline.Start(); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := pipeline.Stop(); err != nil {
		t.Fatalf("Failed to stop pipeline: %v", err)
	}

	// Verify it worked
	stats := pipeline.Stats()
	if stats.Received != 1 {
		t.Errorf("Expected 1 received packet, got %d", stats.Received)
	}
}

func TestPipeline_NoParser(t *testing.T) {
	// Test pipeline without parsers (should still work)
	packets := []core.RawPacket{
		{
			Timestamp:  time.Now(),
			Data:       []byte("packet1"),
			CaptureLen: 7,
			OrigLen:    7,
		},
	}

	pipeline := NewBuilder().
		WithID(4).
		WithTaskID("test-task").
		WithAgentID("test-agent").
		WithCapturer(NewMockCapturer("capturer", packets)).
		WithDecoder(NewMockDecoder()).
		WithReporters(NewMockReporter("reporter")).
		Build()

	if err := pipeline.Start(); err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := pipeline.Stop(); err != nil {
		t.Fatalf("Failed to stop pipeline: %v", err)
	}

	stats := pipeline.Stats()
	if stats.Received != 1 {
		t.Errorf("Expected 1 received packet, got %d", stats.Received)
	}
	// Parsed count depends on whether pipeline counts "raw" as parsed
}


