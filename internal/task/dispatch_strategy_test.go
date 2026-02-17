package task

import (
	"testing"

	"firestige.xyz/otus/internal/core"
)

func TestFlowHashStrategy_Consistency(t *testing.T) {
	s := &FlowHashStrategy{}
	pkt := core.RawPacket{
		Data: makeEthernetUDP("192.168.1.1", "10.0.0.1", 5060, 5060),
	}

	first := s.Dispatch(pkt, 4)
	for i := 0; i < 100; i++ {
		if got := s.Dispatch(pkt, 4); got != first {
			t.Fatalf("FlowHashStrategy not consistent: first=%d, got=%d at iteration %d", first, got, i)
		}
	}
}

func TestFlowHashStrategy_Name(t *testing.T) {
	s := &FlowHashStrategy{}
	if s.Name() != "flow-hash" {
		t.Errorf("expected 'flow-hash', got %q", s.Name())
	}
}

func TestRoundRobinStrategy_Distributes(t *testing.T) {
	s := &RoundRobinStrategy{}
	numPipelines := 3
	counts := make([]int, numPipelines)

	pkt := core.RawPacket{Data: []byte{0x01}}

	for i := 0; i < 30; i++ {
		idx := s.Dispatch(pkt, numPipelines)
		if idx < 0 || idx >= numPipelines {
			t.Fatalf("Dispatch returned out-of-range index: %d", idx)
		}
		counts[idx]++
	}

	for i, c := range counts {
		if c != 10 {
			t.Errorf("pipeline %d received %d packets, expected 10", i, c)
		}
	}
}

func TestRoundRobinStrategy_Name(t *testing.T) {
	s := &RoundRobinStrategy{}
	if s.Name() != "round-robin" {
		t.Errorf("expected 'round-robin', got %q", s.Name())
	}
}

func TestNewDispatchStrategy_FlowHash(t *testing.T) {
	s := NewDispatchStrategy("flow-hash")
	if s.Name() != "flow-hash" {
		t.Errorf("expected flow-hash, got %q", s.Name())
	}
}

func TestNewDispatchStrategy_RoundRobin(t *testing.T) {
	s := NewDispatchStrategy("round-robin")
	if s.Name() != "round-robin" {
		t.Errorf("expected round-robin, got %q", s.Name())
	}
}

func TestNewDispatchStrategy_DefaultFallback(t *testing.T) {
	s := NewDispatchStrategy("")
	if s.Name() != "flow-hash" {
		t.Errorf("empty string should default to flow-hash, got %q", s.Name())
	}

	s2 := NewDispatchStrategy("unknown")
	if s2.Name() != "flow-hash" {
		t.Errorf("unknown strategy should default to flow-hash, got %q", s2.Name())
	}
}

// makeEthernetUDP builds a minimal Ethernet + IPv4 + UDP frame for testing.
func makeEthernetUDP(srcIP, dstIP string, srcPort, dstPort uint16) []byte {
	// Re-use the same test packet construction from flowHash test
	frame := make([]byte, 42) // 14 eth + 20 ip + 8 udp

	// Ethernet header
	frame[12] = 0x08
	frame[13] = 0x00 // IPv4

	// IPv4 header
	frame[14] = 0x45 // version 4, IHL 5
	frame[23] = 17   // protocol UDP

	// IPs - parse manually
	parts := [4]byte{}
	idx := 0
	for _, b := range srcIP {
		if b == '.' {
			idx++
			continue
		}
		parts[idx] = parts[idx]*10 + byte(b-'0')
	}
	copy(frame[26:30], parts[:])

	parts = [4]byte{}
	idx = 0
	for _, b := range dstIP {
		if b == '.' {
			idx++
			continue
		}
		parts[idx] = parts[idx]*10 + byte(b-'0')
	}
	copy(frame[30:34], parts[:])

	// Ports
	frame[34] = byte(srcPort >> 8)
	frame[35] = byte(srcPort)
	frame[36] = byte(dstPort >> 8)
	frame[37] = byte(dstPort)

	return frame
}
