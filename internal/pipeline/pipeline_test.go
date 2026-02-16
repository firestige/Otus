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

// Test cases

func TestPipeline_BasicFlow(t *testing.T) {
	// Create test input packets
	inputChan := make(chan core.RawPacket, 10)
	outputChan := make(chan core.OutputPacket, 10)

	// Create mock components
	decoder := NewMockDecoder()
	parser := NewMockParser("mock-parser", true)
	processor := NewMockProcessor("mock-processor", false)

	// Build pipeline
	pipeline := New(Config{
		ID:         1,
		TaskID:     "test-task",
		AgentID:    "test-agent",
		Decoder:    decoder,
		Parsers:    []plugin.Parser{parser},
		Processors: []plugin.Processor{processor},
	})

	// Start pipeline in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pipeline.Run(ctx, inputChan, outputChan)
	}()

	// Send test packets
	testPackets := []core.RawPacket{
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

	for _, pkt := range testPackets {
		inputChan <- pkt
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Stop pipeline
	cancel()
	close(inputChan)
	wg.Wait()

	// Collect output
	close(outputChan)
	var outputs []core.OutputPacket
	for out := range outputChan {
		outputs = append(outputs, out)
	}

	// Verify we got 2 outputs
	if len(outputs) != 2 {
		t.Errorf("Expected 2 output packets, got %d", len(outputs))
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

	// Verify mock components
	if parser.HandledCount() != 2 {
		t.Errorf("Expected parser to handle 2 packets, got %d", parser.HandledCount())
	}
	if processor.ProcessedCount() != 2 {
		t.Errorf("Expected processor to process 2 packets, got %d", processor.ProcessedCount())
	}
}

func TestPipeline_ProcessorDrop(t *testing.T) {
	// Create channels
	inputChan := make(chan core.RawPacket, 10)
	outputChan := make(chan core.OutputPacket, 10)

	// Create mock components with dropping processor
	decoder := NewMockDecoder()
	parser := NewMockParser("mock-parser", true)
	processor := NewMockProcessor("mock-processor", true) // drops all

	// Build pipeline
	pipeline := New(Config{
		ID:         2,
		TaskID:     "test-task",
		AgentID:    "test-agent",
		Decoder:    decoder,
		Parsers:    []plugin.Parser{parser},
		Processors: []plugin.Processor{processor},
	})

	// Start pipeline
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pipeline.Run(ctx, inputChan, outputChan)
	}()

	// Send test packet
	inputChan <- core.RawPacket{
		Timestamp:  time.Now(),
		Data:       []byte("packet1"),
		CaptureLen: 7,
		OrigLen:    7,
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Stop pipeline
	cancel()
	close(inputChan)
	wg.Wait()

	// Collect output
	close(outputChan)
	var outputs []core.OutputPacket
	for out := range outputChan {
		outputs = append(outputs, out)
	}

	// Verify no output (dropped by processor)
	if len(outputs) != 0 {
		t.Errorf("Expected 0 output packets, got %d", len(outputs))
	}

	// Verify metrics
	stats := pipeline.Stats()
	if stats.Processed != 1 {
		t.Errorf("Expected 1 processed packet, got %d", stats.Processed)
	}
	if stats.Dropped != 1 {
		t.Errorf("Expected 1 dropped packet, got %d", stats.Dropped)
	}
}

func TestBuilder_FluentAPI(t *testing.T) {
	// Build using fluent API
	pipeline := NewBuilder().
		WithID(3).
		WithTaskID("test-task").
		WithAgentID("test-agent").
		WithDecoder(NewMockDecoder()).
		WithParsers(NewMockParser("parser", true)).
		WithProcessors(NewMockProcessor("processor", false)).
		Build()

	// Verify pipeline was created
	if pipeline == nil {
		t.Fatal("Expected non-nil pipeline")
	}
	if pipeline.id != 3 {
		t.Errorf("Expected pipeline ID 3, got %d", pipeline.id)
	}
	if pipeline.taskID != "test-task" {
		t.Errorf("Expected task ID 'test-task', got %s", pipeline.taskID)
	}

	// Run a basic test
	inputChan := make(chan core.RawPacket, 10)
	outputChan := make(chan core.OutputPacket, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pipeline.Run(ctx, inputChan, outputChan)
	}()

	inputChan <- core.RawPacket{
		Timestamp:  time.Now(),
		Data:       []byte("packet1"),
		CaptureLen: 7,
		OrigLen:    7,
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	close(inputChan)
	wg.Wait()

	// Verify it worked
	stats := pipeline.Stats()
	if stats.Received != 1 {
		t.Errorf("Expected 1 received packet, got %d", stats.Received)
	}
}

func TestPipeline_NoParser(t *testing.T) {
	// Test pipeline without parsers (should still work, uses "raw" payload)
	inputChan := make(chan core.RawPacket, 10)
	outputChan := make(chan core.OutputPacket, 10)

	pipeline := NewBuilder().
		WithID(4).
		WithTaskID("test-task").
		WithAgentID("test-agent").
		WithDecoder(NewMockDecoder()).
		Build()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pipeline.Run(ctx, inputChan, outputChan)
	}()

	inputChan <- core.RawPacket{
		Timestamp:  time.Now(),
		Data:       []byte("packet1"),
		CaptureLen: 7,
		OrigLen:    7,
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	close(inputChan)
	wg.Wait()

	close(outputChan)
	var outputs []core.OutputPacket
	for out := range outputChan {
		outputs = append(outputs, out)
	}

	// Should get output with "raw" payload type
	if len(outputs) != 1 {
		t.Errorf("Expected 1 output packet, got %d", len(outputs))
	}
	if len(outputs) > 0 && outputs[0].PayloadType != "raw" {
		t.Errorf("Expected payload type 'raw', got %s", outputs[0].PayloadType)
	}

	stats := pipeline.Stats()
	if stats.Received != 1 {
		t.Errorf("Expected 1 received packet, got %d", stats.Received)
	}
}
