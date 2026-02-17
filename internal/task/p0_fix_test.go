package task

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/internal/core/decoder"
	"firestige.xyz/otus/internal/pipeline"
	"firestige.xyz/otus/pkg/plugin"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// mockReporter is a configurable mock implementing plugin.Reporter.
type mockReporter struct {
	name       string
	startErr   error               // non-nil → Start returns this error
	started    atomic.Bool         // true after Start succeeds
	stopped    atomic.Bool         // set when Stop is called
	reported   []core.OutputPacket // collects packets in Report
	mu         sync.Mutex          // protects reported slice
	reportHook func(ctx context.Context, pkt *core.OutputPacket) error
}

func (m *mockReporter) Name() string                { return m.name }
func (m *mockReporter) Init(_ map[string]any) error { return nil }
func (m *mockReporter) Start(_ context.Context) error {
	if m.startErr != nil {
		return m.startErr
	}
	m.started.Store(true)
	return nil
}
func (m *mockReporter) Stop(_ context.Context) error {
	m.stopped.Store(true)
	return nil
}
func (m *mockReporter) Flush(_ context.Context) error { return nil }
func (m *mockReporter) Report(ctx context.Context, pkt *core.OutputPacket) error {
	if m.reportHook != nil {
		return m.reportHook(ctx, pkt)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reported = append(m.reported, *pkt)
	return nil
}
func (m *mockReporter) packets() []core.OutputPacket {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]core.OutputPacket, len(m.reported))
	copy(cp, m.reported)
	return cp
}

// mockCapturer is a minimal capturer that can be used to satisfy the Task interface.
type mockCapturer struct {
	name    string
	stats   plugin.CaptureStats
	statsMu sync.Mutex
}

func (m *mockCapturer) Name() string                                             { return m.name }
func (m *mockCapturer) Init(_ map[string]any) error                              { return nil }
func (m *mockCapturer) Start(_ context.Context) error                            { return nil }
func (m *mockCapturer) Stop(_ context.Context) error                             { return nil }
func (m *mockCapturer) Capture(_ context.Context, _ chan<- core.RawPacket) error { return nil }
func (m *mockCapturer) Stats() plugin.CaptureStats {
	m.statsMu.Lock()
	defer m.statsMu.Unlock()
	return m.stats
}
func (m *mockCapturer) setStats(recv, drop uint64) {
	m.statsMu.Lock()
	defer m.statsMu.Unlock()
	m.stats = plugin.CaptureStats{PacketsReceived: recv, PacketsDropped: drop}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestTask(reporters []plugin.Reporter, capturers []plugin.Capturer) *Task {
	cfg := config.TaskConfig{
		ID:      "test-p0",
		Workers: 1,
		Capture: config.CaptureConfig{
			Name:         "mock",
			Interface:    "lo",
			DispatchMode: "binding",
			Config:       map[string]any{"interface": "lo"},
		},
		Decoder: config.DecoderConfig{},
	}
	t := NewTask(cfg)
	t.Reporters = reporters
	t.Capturers = capturers

	// Attach a single no-op pipeline so rawStreams[0] has a consumer.
	p := pipeline.New(pipeline.Config{
		ID:      0,
		TaskID:  cfg.ID,
		AgentID: "test-agent",
		Decoder: decoder.NewStandardDecoder(decoder.Config{}),
	})
	t.Pipelines = []*pipeline.Pipeline{p}

	return t
}

// ---------------------------------------------------------------------------
// P0-1: Reporter start failure triggers rollback of already-started reporters
// ---------------------------------------------------------------------------

func TestTask_StartFailureRollback_ThirdReporterFails(t *testing.T) {
	r0 := &mockReporter{name: "r0"}
	r1 := &mockReporter{name: "r1"}
	r2 := &mockReporter{name: "r2", startErr: fmt.Errorf("disk full")}

	cap0 := &mockCapturer{name: "cap0"}

	task := newTestTask(
		[]plugin.Reporter{r0, r1, r2},
		[]plugin.Capturer{cap0},
	)

	err := task.Start()
	if err == nil {
		t.Fatal("expected Start to fail when r2 returns error")
	}

	// r0 and r1 were started then rolled back
	if !r0.started.Load() {
		t.Error("r0 should have been started before rollback")
	}
	if !r1.started.Load() {
		t.Error("r1 should have been started before rollback")
	}
	if r2.started.Load() {
		t.Error("r2 should NOT have been started (it returned error)")
	}

	// Both r0 and r1 should have been stopped (rollback)
	if !r0.stopped.Load() {
		t.Error("r0 should have been stopped during rollback")
	}
	if !r1.stopped.Load() {
		t.Error("r1 should have been stopped during rollback")
	}

	// Task should be in Failed state
	if task.State() != StateFailed {
		t.Errorf("expected state Failed, got %s", task.State())
	}
}

func TestTask_StartFailureRollback_FirstReporterFails(t *testing.T) {
	r0 := &mockReporter{name: "r0", startErr: fmt.Errorf("bind error")}
	r1 := &mockReporter{name: "r1"}

	cap0 := &mockCapturer{name: "cap0"}

	task := newTestTask(
		[]plugin.Reporter{r0, r1},
		[]plugin.Capturer{cap0},
	)

	err := task.Start()
	if err == nil {
		t.Fatal("expected Start to fail when r0 returns error")
	}

	// No reporters should have been started successfully
	if r0.started.Load() {
		t.Error("r0 should NOT have been started (it returned error)")
	}
	if r1.started.Load() {
		t.Error("r1 should NOT have been started (it was never reached)")
	}

	// No rollback needed since nothing was started
	if r0.stopped.Load() {
		t.Error("r0 should NOT have been stopped (nothing to rollback)")
	}
	if r1.stopped.Load() {
		t.Error("r1 should NOT have been stopped (nothing to rollback)")
	}

	if task.State() != StateFailed {
		t.Errorf("expected state Failed, got %s", task.State())
	}
}

// ---------------------------------------------------------------------------
// P0-2: statsCollectorLoop per-capturer delta tracking
// ---------------------------------------------------------------------------

func TestStatsCollector_MultipleCapturers(t *testing.T) {
	// We test the delta logic inline (same algorithm as statsCollectorLoop) because
	// the real goroutine uses a 5-second ticker which is impractical for unit tests.
	type capStats struct {
		packetsReceived uint64
		packetsDropped  uint64
	}

	capturers := []*mockCapturer{
		{name: "c0"},
		{name: "c1"},
		{name: "c2"},
	}

	lastStats := make([]capStats, len(capturers))

	// Simulate tick 1: c0=100, c1=200, c2=50
	capturers[0].setStats(100, 5)
	capturers[1].setStats(200, 10)
	capturers[2].setStats(50, 0)

	var totalDeltaRecv, totalDeltaDrop uint64
	for i, cap := range capturers {
		stats := cap.Stats()
		deltaRecv := stats.PacketsReceived - lastStats[i].packetsReceived
		if stats.PacketsReceived < lastStats[i].packetsReceived {
			deltaRecv = stats.PacketsReceived
		}
		deltaDrop := stats.PacketsDropped - lastStats[i].packetsDropped
		if stats.PacketsDropped < lastStats[i].packetsDropped {
			deltaDrop = stats.PacketsDropped
		}
		totalDeltaRecv += deltaRecv
		totalDeltaDrop += deltaDrop
		lastStats[i] = capStats{packetsReceived: stats.PacketsReceived, packetsDropped: stats.PacketsDropped}
	}

	if totalDeltaRecv != 350 {
		t.Errorf("tick1: expected deltaRecv=350, got %d", totalDeltaRecv)
	}
	if totalDeltaDrop != 15 {
		t.Errorf("tick1: expected deltaDrop=15, got %d", totalDeltaDrop)
	}

	// Simulate tick 2: c0=150, c1=250, c2=50 (c2 unchanged)
	capturers[0].setStats(150, 8)
	capturers[1].setStats(250, 12)
	capturers[2].setStats(50, 0)

	totalDeltaRecv = 0
	totalDeltaDrop = 0
	for i, cap := range capturers {
		stats := cap.Stats()
		deltaRecv := stats.PacketsReceived - lastStats[i].packetsReceived
		if stats.PacketsReceived < lastStats[i].packetsReceived {
			deltaRecv = stats.PacketsReceived
		}
		deltaDrop := stats.PacketsDropped - lastStats[i].packetsDropped
		if stats.PacketsDropped < lastStats[i].packetsDropped {
			deltaDrop = stats.PacketsDropped
		}
		totalDeltaRecv += deltaRecv
		totalDeltaDrop += deltaDrop
		lastStats[i] = capStats{packetsReceived: stats.PacketsReceived, packetsDropped: stats.PacketsDropped}
	}

	// c0: 150-100=50, c1: 250-200=50, c2: 50-50=0 → total=100
	if totalDeltaRecv != 100 {
		t.Errorf("tick2: expected deltaRecv=100, got %d", totalDeltaRecv)
	}
	// c0: 8-5=3, c1: 12-10=2, c2: 0 → total=5
	if totalDeltaDrop != 5 {
		t.Errorf("tick2: expected deltaDrop=5, got %d", totalDeltaDrop)
	}
}

func TestStatsCollector_CounterReset(t *testing.T) {
	// Simulates a counter reset (e.g. capturer restart) where value drops below previous.
	type capStats struct {
		packetsReceived uint64
		packetsDropped  uint64
	}

	cap0 := &mockCapturer{name: "c0"}
	lastStats := []capStats{{packetsReceived: 10000, packetsDropped: 500}}

	// Counter resets: goes from 10000→42
	cap0.setStats(42, 3)

	stats := cap0.Stats()
	deltaRecv := stats.PacketsReceived - lastStats[0].packetsReceived
	if stats.PacketsReceived < lastStats[0].packetsReceived {
		// Underflow protection: treat current value as delta
		deltaRecv = stats.PacketsReceived
	}
	deltaDrop := stats.PacketsDropped - lastStats[0].packetsDropped
	if stats.PacketsDropped < lastStats[0].packetsDropped {
		deltaDrop = stats.PacketsDropped
	}

	if deltaRecv != 42 {
		t.Errorf("expected deltaRecv=42 after reset, got %d", deltaRecv)
	}
	if deltaDrop != 3 {
		t.Errorf("expected deltaDrop=3 after reset, got %d", deltaDrop)
	}
}

// ---------------------------------------------------------------------------
// P0-3: Stop drains sendBuffer before cancelling context
// ---------------------------------------------------------------------------

func TestTask_StopDrainsRemaining(t *testing.T) {
	reporter := &mockReporter{name: "drain-test"}
	cap0 := &mockCapturer{name: "cap0"}

	task := newTestTask(
		[]plugin.Reporter{reporter},
		[]plugin.Capturer{cap0},
	)

	// Start the task fully
	err := task.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Inject packets directly into sendBuffer BEFORE calling Stop.
	// These must be drained by senderLoop before it exits.
	numPackets := 5
	for i := 0; i < numPackets; i++ {
		task.sendBuffer <- core.OutputPacket{
			TaskID:      "test-p0",
			PayloadType: "test",
			SrcPort:     uint16(i),
		}
	}

	// Stop should drain all injected packets through senderLoop → reporter
	err = task.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	received := reporter.packets()
	if len(received) != numPackets {
		t.Errorf("expected reporter to receive %d packets, got %d", numPackets, len(received))
	}

	// Verify packet content (ordering preserved since single sender)
	for i, pkt := range received {
		if pkt.SrcPort != uint16(i) {
			t.Errorf("packet[%d]: expected SrcPort=%d, got %d", i, i, pkt.SrcPort)
		}
	}

	if task.State() != StateStopped {
		t.Errorf("expected state Stopped, got %s", task.State())
	}
}

func TestTask_StopContextValidDuringSend(t *testing.T) {
	// Verify that ctx is NOT cancelled while senderLoop is still draining.
	// Reporter.Report receives a valid (non-cancelled) context.
	var ctxWasCancelled atomic.Bool

	reporter := &mockReporter{
		name: "ctx-check",
		reportHook: func(ctx context.Context, pkt *core.OutputPacket) error {
			if ctx.Err() != nil {
				ctxWasCancelled.Store(true)
			}
			return nil
		},
	}
	cap0 := &mockCapturer{name: "cap0"}

	task := newTestTask(
		[]plugin.Reporter{reporter},
		[]plugin.Capturer{cap0},
	)

	err := task.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Inject a packet
	task.sendBuffer <- core.OutputPacket{TaskID: "test-p0", PayloadType: "test"}

	// Give senderLoop a moment to pick it up before Stop is called.
	// In practice Stop closes sendBuffer which causes senderLoop drain.
	time.Sleep(10 * time.Millisecond)

	err = task.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if ctxWasCancelled.Load() {
		t.Error("context should NOT be cancelled while senderLoop is draining")
	}
}

// ---------------------------------------------------------------------------
// Edge case: Start cannot be called twice
// ---------------------------------------------------------------------------

func TestTask_DoubleStart(t *testing.T) {
	reporter := &mockReporter{name: "r"}
	cap0 := &mockCapturer{name: "cap0"}

	task := newTestTask(
		[]plugin.Reporter{reporter},
		[]plugin.Capturer{cap0},
	)

	if err := task.Start(); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer task.Stop()

	err := task.Start()
	if err == nil {
		t.Error("expected second Start to fail")
	}
}
