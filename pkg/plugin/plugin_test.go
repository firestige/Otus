package plugin

import (
	"context"
	"testing"

	"firestige.xyz/otus/internal/core"
)

// Mock implementations for testing interface compliance

type mockPlugin struct {
	name        string
	initErr     error
	startErr    error
	stopErr     error
	initCalled  bool
	startCalled bool
	stopCalled  bool
}

func (m *mockPlugin) Name() string {
	return m.name
}

func (m *mockPlugin) Init(cfg map[string]any) error {
	m.initCalled = true
	return m.initErr
}

func (m *mockPlugin) Start(ctx context.Context) error {
	m.startCalled = true
	return m.startErr
}

func (m *mockPlugin) Stop(ctx context.Context) error {
	m.stopCalled = true
	return m.stopErr
}

// Test Plugin interface
func TestPluginInterface(t *testing.T) {
	t.Run("BasicLifecycle", func(t *testing.T) {
		mock := &mockPlugin{name: "test-plugin"}

		if mock.Name() != "test-plugin" {
			t.Errorf("expected name 'test-plugin', got %s", mock.Name())
		}

		cfg := map[string]any{"key": "value"}
		if err := mock.Init(cfg); err != nil {
			t.Errorf("Init failed: %v", err)
		}
		if !mock.initCalled {
			t.Error("Init was not called")
		}

		ctx := context.Background()
		if err := mock.Start(ctx); err != nil {
			t.Errorf("Start failed: %v", err)
		}
		if !mock.startCalled {
			t.Error("Start was not called")
		}

		if err := mock.Stop(ctx); err != nil {
			t.Errorf("Stop failed: %v", err)
		}
		if !mock.stopCalled {
			t.Error("Stop was not called")
		}
	})
}

// Mock Capturer
type mockCapturer struct {
	mockPlugin
	captureErr error
	stats      CaptureStats
}

func (m *mockCapturer) Capture(ctx context.Context, output chan<- core.RawPacket) error {
	return m.captureErr
}

func (m *mockCapturer) Stats() CaptureStats {
	return m.stats
}

func TestCapturerInterface(t *testing.T) {
	t.Run("CaptureAndStats", func(t *testing.T) {
		mock := &mockCapturer{
			mockPlugin: mockPlugin{name: "mock-capturer"},
			stats: CaptureStats{
				PacketsReceived:  1000,
				PacketsDropped:   10,
				PacketsIfDropped: 5,
			},
		}

		// Test interface compliance
		var _ Capturer = mock

		stats := mock.Stats()
		if stats.PacketsReceived != 1000 {
			t.Errorf("expected PacketsReceived=1000, got %d", stats.PacketsReceived)
		}
		if stats.PacketsDropped != 10 {
			t.Errorf("expected PacketsDropped=10, got %d", stats.PacketsDropped)
		}
		if stats.PacketsIfDropped != 5 {
			t.Errorf("expected PacketsIfDropped=5, got %d", stats.PacketsIfDropped)
		}

		ctx := context.Background()
		output := make(chan core.RawPacket, 1)
		if err := mock.Capture(ctx, output); err != nil {
			t.Errorf("Capture failed: %v", err)
		}
	})
}

// Mock Parser
type mockParser struct {
	mockPlugin
	canHandle   bool
	handleErr   error
	parsedData  any
	parsedLabel core.Labels
}

func (m *mockParser) CanHandle(pkt *core.DecodedPacket) bool {
	return m.canHandle
}

func (m *mockParser) Handle(pkt *core.DecodedPacket) (payload any, labels core.Labels, err error) {
	return m.parsedData, m.parsedLabel, m.handleErr
}

func TestParserInterface(t *testing.T) {
	t.Run("CanHandleAndHandle", func(t *testing.T) {
		labels := make(core.Labels)
		labels[core.LabelSIPMethod] = "INVITE"

		mock := &mockParser{
			mockPlugin:  mockPlugin{name: "mock-parser"},
			canHandle:   true,
			parsedData:  "parsed payload",
			parsedLabel: labels,
		}

		// Test interface compliance
		var _ Parser = mock

		pkt := &core.DecodedPacket{}
		if !mock.CanHandle(pkt) {
			t.Error("expected CanHandle to return true")
		}

		payload, retLabels, err := mock.Handle(pkt)
		if err != nil {
			t.Errorf("Handle failed: %v", err)
		}
		if payload != "parsed payload" {
			t.Errorf("expected 'parsed payload', got %v", payload)
		}
		if retLabels[core.LabelSIPMethod] != "INVITE" {
			t.Errorf("expected INVITE label, got %s", retLabels[core.LabelSIPMethod])
		}
	})
}

// Mock Processor
type mockProcessor struct {
	mockPlugin
	shouldKeep bool
}

func (m *mockProcessor) Process(pkt *core.OutputPacket) bool {
	return m.shouldKeep
}

func TestProcessorInterface(t *testing.T) {
	t.Run("ProcessKeep", func(t *testing.T) {
		mock := &mockProcessor{
			mockPlugin: mockPlugin{name: "mock-processor"},
			shouldKeep: true,
		}

		// Test interface compliance
		var _ Processor = mock

		pkt := &core.OutputPacket{}
		if !mock.Process(pkt) {
			t.Error("expected Process to return true")
		}
	})

	t.Run("ProcessDrop", func(t *testing.T) {
		mock := &mockProcessor{
			mockPlugin: mockPlugin{name: "mock-processor"},
			shouldKeep: false,
		}

		pkt := &core.OutputPacket{}
		if mock.Process(pkt) {
			t.Error("expected Process to return false")
		}
	})
}

// Mock Reporter
type mockReporter struct {
	mockPlugin
	reportErr error
	flushErr  error
	reported  []*core.OutputPacket
}

func (m *mockReporter) Report(ctx context.Context, pkt *core.OutputPacket) error {
	m.reported = append(m.reported, pkt)
	return m.reportErr
}

func (m *mockReporter) Flush(ctx context.Context) error {
	return m.flushErr
}

func TestReporterInterface(t *testing.T) {
	t.Run("ReportAndFlush", func(t *testing.T) {
		mock := &mockReporter{
			mockPlugin: mockPlugin{name: "mock-reporter"},
		}

		// Test interface compliance
		var _ Reporter = mock

		ctx := context.Background()
		pkt := &core.OutputPacket{TaskID: "task-001"}

		if err := mock.Report(ctx, pkt); err != nil {
			t.Errorf("Report failed: %v", err)
		}

		if len(mock.reported) != 1 {
			t.Errorf("expected 1 reported packet, got %d", len(mock.reported))
		}
		if mock.reported[0].TaskID != "task-001" {
			t.Errorf("expected TaskID=task-001, got %s", mock.reported[0].TaskID)
		}

		if err := mock.Flush(ctx); err != nil {
			t.Errorf("Flush failed: %v", err)
		}
	})
}

// Test interface embedding
func TestInterfaceEmbedding(t *testing.T) {
	t.Run("CapturerEmbedsPlugin", func(t *testing.T) {
		mock := &mockCapturer{
			mockPlugin: mockPlugin{name: "test"},
		}

		// Should be able to call Plugin methods
		if mock.Name() != "test" {
			t.Error("Capturer should embed Plugin interface")
		}
	})

	t.Run("ParserEmbedsPlugin", func(t *testing.T) {
		mock := &mockParser{
			mockPlugin: mockPlugin{name: "test"},
		}

		if mock.Name() != "test" {
			t.Error("Parser should embed Plugin interface")
		}
	})

	t.Run("ProcessorEmbedsPlugin", func(t *testing.T) {
		mock := &mockProcessor{
			mockPlugin: mockPlugin{name: "test"},
		}

		if mock.Name() != "test" {
			t.Error("Processor should embed Plugin interface")
		}
	})

	t.Run("ReporterEmbedsPlugin", func(t *testing.T) {
		mock := &mockReporter{
			mockPlugin: mockPlugin{name: "test"},
		}

		if mock.Name() != "test" {
			t.Error("Reporter should embed Plugin interface")
		}
	})
}
