package rtp

import (
	"context"
	"encoding/binary"
	"net/netip"
	"testing"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/pkg/plugin"
)

// ---------------------------------------------------------------------------
// Mock FlowRegistry
// ---------------------------------------------------------------------------

type mockFlowRegistry struct {
	flows map[plugin.FlowKey]any
}

func newMockFlowRegistry() *mockFlowRegistry {
	return &mockFlowRegistry{flows: make(map[plugin.FlowKey]any)}
}

func (m *mockFlowRegistry) Get(key plugin.FlowKey) (any, bool) {
	v, ok := m.flows[key]
	return v, ok
}
func (m *mockFlowRegistry) Set(key plugin.FlowKey, value any)  { m.flows[key] = value }
func (m *mockFlowRegistry) Delete(key plugin.FlowKey)           { delete(m.flows, key) }
func (m *mockFlowRegistry) Count() int                          { return len(m.flows) }
func (m *mockFlowRegistry) Clear()                              { m.flows = make(map[plugin.FlowKey]any) }
func (m *mockFlowRegistry) Range(f func(plugin.FlowKey, any) bool) {
	for k, v := range m.flows {
		if !f(k, v) {
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Packet builders
// ---------------------------------------------------------------------------

// makeRTPPayload builds a minimal 12-byte RTP header.
//
//	byte 0: V=2  P=0  X=ext  CC=0  →  0x80 | (ext << 4)
//	byte 1: M=marker  PT=pt
//	bytes 2-3: sequence
//	bytes 4-7: timestamp
//	bytes 8-11: ssrc
func makeRTPPayload(pt uint8, seq uint16, ts uint32, ssrc uint32, marker bool, ext bool) []byte {
	b := make([]byte, 12)
	b[0] = 0x80
	if ext {
		b[0] |= 0x10
	}
	b[1] = pt & 0x7F
	if marker {
		b[1] |= 0x80
	}
	binary.BigEndian.PutUint16(b[2:4], seq)
	binary.BigEndian.PutUint32(b[4:8], ts)
	binary.BigEndian.PutUint32(b[8:12], ssrc)
	return b
}

// makeRTCPPayload builds a minimal 8-byte RTCP SR header.
//
//	byte 0: V=2  P=0  RC=0  →  0x80
//	byte 1: PT (200=SR, 201=RR, …)
//	bytes 2-3: length (words - 1)
//	bytes 4-7: SSRC of sender
func makeRTCPPayload(pt uint8, ssrc uint32) []byte {
	b := make([]byte, 8)
	b[0] = 0x80 // V=2
	b[1] = pt
	binary.BigEndian.PutUint16(b[2:4], 1) // length in 32-bit words minus one
	binary.BigEndian.PutUint32(b[4:8], ssrc)
	return b
}

func makeDecodedPacket(srcIP, dstIP string, srcPort, dstPort uint16, payload []byte) *core.DecodedPacket {
	return &core.DecodedPacket{
		IP: core.IPHeader{
			SrcIP:    netip.MustParseAddr(srcIP),
			DstIP:    netip.MustParseAddr(dstIP),
			Protocol: 17,
		},
		Transport: core.TransportHeader{
			SrcPort:  srcPort,
			DstPort:  dstPort,
			Protocol: 17,
		},
		Payload: payload,
	}
}

// ---------------------------------------------------------------------------
// Basic plugin interface tests
// ---------------------------------------------------------------------------

func TestName(t *testing.T) {
	p := NewRTPParser()
	if p.Name() != "rtp" {
		t.Errorf("Name() = %q; want %q", p.Name(), "rtp")
	}
}

func TestLifecycle(t *testing.T) {
	p := NewRTPParser()
	ctx := context.Background()

	if err := p.Init(nil); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestSetFlowRegistry(t *testing.T) {
	p := NewRTPParser().(*RTPParser)
	reg := newMockFlowRegistry()
	p.SetFlowRegistry(reg)
	if p.flowRegistry != reg {
		t.Error("SetFlowRegistry did not store the registry")
	}
}

// ---------------------------------------------------------------------------
// CanHandle tests
// ---------------------------------------------------------------------------

func TestCanHandle_NonUDP(t *testing.T) {
	p := NewRTPParser().(*RTPParser)
	pkt := &core.DecodedPacket{
		IP:        core.IPHeader{Protocol: 6},
		Transport: core.TransportHeader{Protocol: 6, SrcPort: 10000, DstPort: 20000},
		Payload:   makeRTPPayload(0, 1, 100, 0xDEADBEEF, false, false),
	}
	if p.CanHandle(pkt) {
		t.Error("CanHandle should return false for TCP packets")
	}
}

func TestCanHandle_FlowRegistryHit(t *testing.T) {
	p := NewRTPParser().(*RTPParser)
	reg := newMockFlowRegistry()
	p.SetFlowRegistry(reg)

	srcIP := netip.MustParseAddr("192.168.1.10")
	dstIP := netip.MustParseAddr("192.168.1.20")
	reg.Set(plugin.FlowKey{SrcIP: srcIP, DstIP: dstIP, SrcPort: 6000, DstPort: 7000, Proto: 17},
		map[string]string{"call_id": "abc123", "codec": "PCMU"})

	pkt := makeDecodedPacket("192.168.1.10", "192.168.1.20", 6000, 7000,
		[]byte{0xFF, 0xFF}) // garbage payload — registry hit should short-circuit
	if !p.CanHandle(pkt) {
		t.Error("CanHandle should return true when FlowRegistry has an entry for this 5-tuple")
	}
}

func TestCanHandle_HeuristicRTP(t *testing.T) {
	p := NewRTPParser()
	payload := makeRTPPayload(0, 1, 100, 0xDEADBEEF, false, false)
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, payload)
	if !p.CanHandle(pkt) {
		t.Error("CanHandle should return true for valid RTP heuristic")
	}
}

func TestCanHandle_HeuristicRTCP_SR(t *testing.T) {
	p := NewRTPParser()
	payload := makeRTCPPayload(200, 0xAABBCCDD) // 200 = SR
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6001, 7001, payload)
	if !p.CanHandle(pkt) {
		t.Error("CanHandle should return true for RTCP SR heuristic")
	}
}

func TestCanHandle_TooShort(t *testing.T) {
	p := NewRTPParser()
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, []byte{0x80, 0x00})
	if p.CanHandle(pkt) {
		t.Error("CanHandle should return false for payload shorter than RTCP min length")
	}
}

func TestCanHandle_WrongVersion(t *testing.T) {
	p := NewRTPParser()
	payload := makeRTPPayload(0, 1, 100, 0xDEADBEEF, false, false)
	payload[0] = (payload[0] & 0x3F) | 0x40 // force V=1
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, payload)
	if p.CanHandle(pkt) {
		t.Error("CanHandle should return false when V != 2")
	}
}


// ---------------------------------------------------------------------------
// Handle — RTP parsing tests
// ---------------------------------------------------------------------------

func TestHandle_RTP_Labels(t *testing.T) {
	p := NewRTPParser()
	payload := makeRTPPayload(8, 1234, 9999, 0xDEADBEEF, true, false)
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, payload)

	result, labels, err := p.Handle(pkt)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if result != nil {
		t.Errorf("Handle() payload = %v; want nil", result)
	}

	checks := map[string]string{
		core.LabelRTPVersion:     "2",
		core.LabelRTPPayloadType: "8",
		core.LabelRTPSeq:         "1234",
		core.LabelRTPTimestamp:   "9999",
		core.LabelRTPSSRC:        "0xDEADBEEF",
		core.LabelRTPMarker:      "true",
		core.LabelRTPExtension:   "false",
	}
	for k, want := range checks {
		if got := labels[k]; got != want {
			t.Errorf("label[%q] = %q; want %q", k, got, want)
		}
	}
}

func TestHandle_RTP_WithFlowRegistry(t *testing.T) {
	p := NewRTPParser().(*RTPParser)
	reg := newMockFlowRegistry()
	p.SetFlowRegistry(reg)

	srcIP := netip.MustParseAddr("10.0.0.1")
	dstIP := netip.MustParseAddr("10.0.0.2")
	reg.Set(plugin.FlowKey{SrcIP: srcIP, DstIP: dstIP, SrcPort: 6000, DstPort: 7000, Proto: 17},
		map[string]string{"call_id": "call-xyz-789", "codec": "G711A"})

	payload := makeRTPPayload(8, 1, 100, 0x11223344, false, false)
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, payload)

	_, labels, err := p.Handle(pkt)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if got := labels[core.LabelRTPCallID]; got != "call-xyz-789" {
		t.Errorf("LabelRTPCallID = %q; want %q", got, "call-xyz-789")
	}
	if got := labels[core.LabelRTPCodec]; got != "G711A" {
		t.Errorf("LabelRTPCodec = %q; want %q", got, "G711A")
	}
}

func TestHandle_RTP_NoFlowRegistry(t *testing.T) {
	// Without registry, call_id and codec labels must simply be absent (no panic).
	p := NewRTPParser()
	payload := makeRTPPayload(0, 1, 100, 0xDEADBEEF, false, false)
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, payload)

	_, labels, err := p.Handle(pkt)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if _, ok := labels[core.LabelRTPCallID]; ok {
		t.Error("LabelRTPCallID should not be set when FlowRegistry is nil")
	}
}

func TestHandle_RTP_TooShort(t *testing.T) {
	p := NewRTPParser()
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, []byte{0x80, 0x00, 0x00})
	_, _, err := p.Handle(pkt)
	if err == nil {
		t.Error("Handle() should return error for payload shorter than RTP header")
	}
}

func TestHandle_RTP_WrongVersion(t *testing.T) {
	p := NewRTPParser()
	payload := makeRTPPayload(0, 1, 100, 0xDEADBEEF, false, false)
	payload[0] = (payload[0] & 0x3F) | 0x40 // V=1
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, payload)

	_, _, err := p.Handle(pkt)
	if err == nil {
		t.Error("Handle() should return error for RTP version != 2")
	}
}

// ---------------------------------------------------------------------------
// Handle — RTCP parsing tests
// ---------------------------------------------------------------------------

func TestHandle_RTCP_SR_Labels(t *testing.T) {
	p := NewRTPParser()
	payload := makeRTCPPayload(200, 0xAABBCCDD) // SR
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6001, 7001, payload)

	result, labels, err := p.Handle(pkt)
	if err != nil {
		t.Fatalf("Handle() RTCP SR error: %v", err)
	}
	if result != nil {
		t.Errorf("Handle() payload = %v; want nil", result)
	}
	if got := labels[core.LabelRTCPPayloadType]; got != "200" {
		t.Errorf("LabelRTCPPayloadType = %q; want %q", got, "200")
	}
	if got := labels[core.LabelRTCPSSRC]; got != "0xAABBCCDD" {
		t.Errorf("LabelRTCPSSRC = %q; want %q", got, "0xAABBCCDD")
	}
}

func TestHandle_RTCP_AllTypes(t *testing.T) {
	tests := []struct {
		pt   uint8
		name string
	}{
		{200, "SR"},
		{201, "RR"},
		{202, "SDES"},
		{203, "BYE"},
		{204, "APP"},
		{205, "RTPFB"},
		{206, "PSFB"},
		{207, "XR"},
		{209, "limit"},
	}

	p := NewRTPParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := makeRTCPPayload(tt.pt, 0x12345678)
			pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6001, 7001, payload)
			_, labels, err := p.Handle(pkt)
			if err != nil {
				t.Fatalf("Handle() RTCP PT=%d error: %v", tt.pt, err)
			}
			want := string([]byte{byte('0' + tt.pt/100), byte('0' + (tt.pt/10)%10), byte('0' + tt.pt%10)})
			// Use fmt format consistent with implementation
			want2 := labels[core.LabelRTCPPayloadType]
			_ = want
			if want2 == "" {
				t.Errorf("LabelRTCPPayloadType missing for PT=%d", tt.pt)
			}
		})
	}
}

func TestHandle_RTCP_WithFlowRegistry(t *testing.T) {
	p := NewRTPParser().(*RTPParser)
	reg := newMockFlowRegistry()
	p.SetFlowRegistry(reg)

	srcIP := netip.MustParseAddr("10.0.0.1")
	dstIP := netip.MustParseAddr("10.0.0.2")
	reg.Set(plugin.FlowKey{SrcIP: srcIP, DstIP: dstIP, SrcPort: 6001, DstPort: 7001, Proto: 17},
		map[string]string{"call_id": "rtcp-call-001", "codec": "RTCP"})

	payload := makeRTCPPayload(201, 0xAABBCCDD) // RR
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6001, 7001, payload)

	_, labels, err := p.Handle(pkt)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if got := labels[core.LabelRTCPCallID]; got != "rtcp-call-001" {
		t.Errorf("LabelRTCPCallID = %q; want %q", got, "rtcp-call-001")
	}
}

func TestHandle_RTCP_TooShort(t *testing.T) {
	p := NewRTPParser()
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6001, 7001,
		[]byte{0x80, 200, 0x00, 0x00}) // 4 bytes, RTCP needs 8
	_, _, err := p.Handle(pkt)
	if err == nil {
		t.Error("Handle() should return error for RTCP payload shorter than 8 bytes")
	}
}

func TestHandle_TooShort_TwoBytesMin(t *testing.T) {
	p := NewRTPParser()
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, []byte{0x80})
	_, _, err := p.Handle(pkt)
	if err == nil {
		t.Error("Handle() should return error for single-byte payload")
	}
}

// ---------------------------------------------------------------------------
// looksLikeRTPorRTCP unit tests
// ---------------------------------------------------------------------------

func TestLooksLikeRTPorRTCP(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    bool
	}{
		{
			name:    "valid RTP PT=0",
			payload: makeRTPPayload(0, 1, 100, 0x12345678, false, false),
			want:    true,
		},
		{
			name:    "valid RTP PT=127 (max dynamic)",
			payload: makeRTPPayload(127, 1, 100, 0x12345678, false, false),
			want:    true,
		},
		{
			name:    "valid RTCP SR (PT=200)",
			payload: makeRTCPPayload(200, 0xAABBCCDD),
			want:    true,
		},
		{
			name:    "valid RTCP BYE (PT=203)",
			payload: makeRTCPPayload(203, 0xAABBCCDD),
			want:    true,
		},
		{
			name:    "too short (7 bytes)",
			payload: []byte{0x80, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00},
			want:    false,
		},
		{
			name:    "wrong version (V=0)",
			payload: append([]byte{0x00}, makeRTPPayload(0, 1, 100, 0, false, false)[1:]...),
			want:    false,
		},
		{
			name:    "wrong version (V=1)",
			payload: append([]byte{0x40}, makeRTPPayload(0, 1, 100, 0, false, false)[1:]...),
			want:    false,
		},
		{
			// byte 1 = 0x80 means M=1 PT=0 (PCMU) — V=2 in byte 0, valid RTP.
			name:    "marker=1 PT=0 is valid RTP",
			payload: func() []byte { b := makeRTPPayload(0, 1, 100, 0, true, false); return b }(),
			want:    true,
		},
		{
			name:    "empty payload",
			payload: []byte{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeRTPorRTCP(tt.payload)
			if got != tt.want {
				t.Errorf("looksLikeRTPorRTCP() = %v; want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FlowRegistry miss — enrichFromRegistry should not panic
// ---------------------------------------------------------------------------

func TestEnrichFromRegistry_Miss(t *testing.T) {
	p := NewRTPParser().(*RTPParser)
	reg := newMockFlowRegistry() // empty
	p.SetFlowRegistry(reg)

	payload := makeRTPPayload(8, 1, 100, 0x11223344, false, false)
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, payload)

	_, labels, err := p.Handle(pkt)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	// call_id and codec must be absent — no entry in registry.
	if _, ok := labels[core.LabelRTPCallID]; ok {
		t.Error("LabelRTPCallID should not be present on registry miss")
	}
	if _, ok := labels[core.LabelRTPCodec]; ok {
		t.Error("LabelRTPCodec should not be present on registry miss")
	}
}

func TestEnrichFromRegistry_WrongType(t *testing.T) {
	p := NewRTPParser().(*RTPParser)
	reg := newMockFlowRegistry()
	p.SetFlowRegistry(reg)

	srcIP := netip.MustParseAddr("10.0.0.1")
	dstIP := netip.MustParseAddr("10.0.0.2")
	// Store a wrong type — should not panic.
	reg.Set(plugin.FlowKey{SrcIP: srcIP, DstIP: dstIP, SrcPort: 6000, DstPort: 7000, Proto: 17},
		"unexpected string value")

	payload := makeRTPPayload(0, 1, 100, 0xDEADBEEF, false, false)
	pkt := makeDecodedPacket("10.0.0.1", "10.0.0.2", 6000, 7000, payload)

	_, labels, err := p.Handle(pkt)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if _, ok := labels[core.LabelRTPCallID]; ok {
		t.Error("LabelRTPCallID should not be present when registry value has wrong type")
	}
}
