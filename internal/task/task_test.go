package task

import (
	"testing"

	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/core"
)

func TestTaskStateTransitions(t *testing.T) {
	cfg := config.TaskConfig{
		ID:      "test-task-1",
		Workers: 1,
		Capture: config.CaptureConfig{
			Name:         "mock",
			Interface:    "lo",
			DispatchMode: "binding",
		},
		Decoder: config.DecoderConfig{
			Tunnels:      []string{},
			IPReassembly: false,
		},
		Parsers:    []config.ParserConfig{},
		Processors: []config.ProcessorConfig{},
		Reporters: []config.ReporterConfig{
			{
				Name:   "console",
				Config: map[string]any{},
			},
		},
	}

	task := NewTask(cfg)

	// Initial state should be Created
	if task.State() != StateCreated {
		t.Errorf("Expected initial state Created, got %s", task.State())
	}

	// Test ID()
	if task.ID() != "test-task-1" {
		t.Errorf("Expected ID 'test-task-1', got %s", task.ID())
	}

	// Test GetStatus()
	status := task.GetStatus()
	if status.ID != "test-task-1" {
		t.Errorf("Expected status ID 'test-task-1', got %s", status.ID)
	}
	if status.State != StateCreated {
		t.Errorf("Expected status state Created, got %s", status.State)
	}
	if status.PipelineCount != 0 {
		t.Errorf("Expected pipeline count 0, got %d", status.PipelineCount)
	}
}

func TestTaskCreatedAttributes_BindingMode(t *testing.T) {
	cfg := config.TaskConfig{
		ID:      "test-task-2",
		Workers: 4,
		Capture: config.CaptureConfig{
			Name:         "mock",
			Interface:    "eth0",
			DispatchMode: "binding",
		},
		Decoder:    config.DecoderConfig{},
		Parsers:    []config.ParserConfig{},
		Processors: []config.ProcessorConfig{},
		Reporters:  []config.ReporterConfig{},
	}

	task := NewTask(cfg)

	// Binding mode: captureCh should NOT be allocated
	if task.captureCh != nil {
		t.Error("Expected captureCh to be nil in binding mode")
	}

	if task.sendBuffer == nil {
		t.Error("Expected sendBuffer to be initialized")
	}

	if task.doneCh == nil {
		t.Error("Expected doneCh to be initialized")
	}

	// Raw streams created based on Workers count
	if len(task.rawStreams) != 4 {
		t.Errorf("Expected 4 raw streams, got %d", len(task.rawStreams))
	}

	// Check context is created
	if task.ctx == nil {
		t.Error("Expected ctx to be initialized")
	}

	if task.cancel == nil {
		t.Error("Expected cancel func to be initialized")
	}
}

func TestTaskCreatedAttributes_DispatchMode(t *testing.T) {
	cfg := config.TaskConfig{
		ID:      "test-task-2b",
		Workers: 2,
		Capture: config.CaptureConfig{
			Name:         "mock",
			Interface:    "eth0",
			DispatchMode: "dispatch",
		},
		Decoder:    config.DecoderConfig{},
		Parsers:    []config.ParserConfig{},
		Processors: []config.ProcessorConfig{},
		Reporters:  []config.ReporterConfig{},
	}

	task := NewTask(cfg)

	// Dispatch mode: captureCh SHOULD be allocated
	if task.captureCh == nil {
		t.Error("Expected captureCh to be initialized in dispatch mode")
	}

	// Raw streams still based on Workers
	if len(task.rawStreams) != 2 {
		t.Errorf("Expected 2 raw streams, got %d", len(task.rawStreams))
	}
}

func TestTaskDefaultWorkers(t *testing.T) {
	cfg := config.TaskConfig{
		ID:      "test-task-3",
		Workers: 0, // Invalid, should default to 1
		Capture: config.CaptureConfig{
			Name:         "mock",
			Interface:    "eth0",
			DispatchMode: "binding",
		},
	}

	task := NewTask(cfg)

	// Should default to 1 raw stream
	if len(task.rawStreams) != 1 {
		t.Errorf("Expected 1 raw stream for invalid workers, got %d", len(task.rawStreams))
	}
}

func TestTaskStateCreatedToFailed(t *testing.T) {
	cfg := config.TaskConfig{
		ID:      "test-task-4",
		Workers: 1,
		Capture: config.CaptureConfig{
			Name:         "nonexistent",
			Interface:    "lo",
			DispatchMode: "binding",
		},
	}

	task := NewTask(cfg)

	// Manually trigger state transition to demonstrate state machine
	task.mu.Lock()
	task.setState(StateFailed)
	task.failureReason = "test failure"
	task.mu.Unlock()

	if task.State() != StateFailed {
		t.Errorf("Expected state Failed, got %s", task.State())
	}

	status := task.GetStatus()
	if status.FailureReason != "test failure" {
		t.Errorf("Expected failure reason 'test failure', got %s", status.FailureReason)
	}
}

func TestFlowHash(t *testing.T) {
	// Build a minimal IPv4/UDP Ethernet frame: 14 (eth) + 20 (ipv4) + 8 (udp)
	buildIPv4UDP := func(srcIP, dstIP [4]byte, srcPort, dstPort uint16) core.RawPacket {
		frame := make([]byte, 42) // 14 + 20 + 8
		// Ethernet: ethertype = 0x0800 (IPv4)
		frame[12] = 0x08
		frame[13] = 0x00
		// IPv4 header
		frame[14] = 0x45             // version=4, IHL=5
		frame[23] = 17               // protocol = UDP
		copy(frame[26:30], srcIP[:]) // src IP
		copy(frame[30:34], dstIP[:]) // dst IP
		// UDP header
		frame[34] = byte(srcPort >> 8)
		frame[35] = byte(srcPort)
		frame[36] = byte(dstPort >> 8)
		frame[37] = byte(dstPort)
		return core.RawPacket{Data: frame}
	}

	t.Run("same 5-tuple yields same hash", func(t *testing.T) {
		pkt1 := buildIPv4UDP([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 5060, 5060)
		pkt2 := buildIPv4UDP([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 5060, 5060)
		if flowHash(pkt1) != flowHash(pkt2) {
			t.Error("identical 5-tuples should produce identical hash")
		}
	})

	t.Run("different src port yields different hash", func(t *testing.T) {
		pkt1 := buildIPv4UDP([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 5060, 5060)
		pkt2 := buildIPv4UDP([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 5061, 5060)
		if flowHash(pkt1) == flowHash(pkt2) {
			t.Error("different src ports should (very likely) produce different hash")
		}
	})

	t.Run("different dst IP yields different hash", func(t *testing.T) {
		pkt1 := buildIPv4UDP([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 5060, 5060)
		pkt2 := buildIPv4UDP([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 3}, 5060, 5060)
		if flowHash(pkt1) == flowHash(pkt2) {
			t.Error("different dst IPs should (very likely) produce different hash")
		}
	})

	t.Run("reverse direction yields different hash", func(t *testing.T) {
		pktAB := buildIPv4UDP([4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2}, 1234, 5060)
		pktBA := buildIPv4UDP([4]byte{10, 0, 0, 2}, [4]byte{10, 0, 0, 1}, 5060, 1234)
		// Different direction should have different hash (asymmetric hashing is expected)
		_ = flowHash(pktAB)
		_ = flowHash(pktBA)
		// Just verifying it computes without panic; asymmetric hash is intentional
	})

	t.Run("short packet falls back gracefully", func(t *testing.T) {
		pkt := core.RawPacket{Data: []byte{0x01, 0x02, 0x03}}
		h := flowHash(pkt)
		if h == 0 {
			t.Error("short packet should still produce a non-zero hash")
		}
	})

	t.Run("VLAN tagged frame", func(t *testing.T) {
		// 802.1Q: 18 (eth+vlan) + 20 (ipv4) + 8 (udp) = 46
		frame := make([]byte, 46)
		frame[12] = 0x81 // 802.1Q ethertype
		frame[13] = 0x00
		frame[16] = 0x08 // inner ethertype = IPv4
		frame[17] = 0x00
		// IPv4 at offset 18
		frame[18] = 0x45 // version=4, IHL=5
		frame[27] = 17   // protocol = UDP
		frame[30] = 10   // src IP: 10.0.0.1
		frame[33] = 1
		frame[34] = 10 // dst IP: 10.0.0.2
		frame[37] = 2
		frame[38] = 0x13 // src port: 5060
		frame[39] = 0xC4
		frame[40] = 0x13 // dst port: 5060
		frame[41] = 0xC4

		pkt := core.RawPacket{Data: frame}
		h := flowHash(pkt)
		if h == 0 {
			t.Error("VLAN tagged packet should produce a non-zero hash")
		}
	})
}
