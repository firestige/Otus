package hep

import (
	"context"
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"

	"firestige.xyz/otus/internal/core"
)

// ─── Encoder tests ─────────────────────────────────────────────────────────

// parsedChunks holds the decoded content of a HEPv3 frame for assertions.
type parsedFrame struct {
	magic  string
	length uint16
	chunks map[uint16][]byte // chunkType → raw value bytes
}

// parseFrame decodes a raw HEPv3 byte slice for test assertions.
func parseFrame(t *testing.T, data []byte) parsedFrame {
	t.Helper()

	if len(data) < 6 {
		t.Fatalf("frame too short: %d bytes", len(data))
	}

	pf := parsedFrame{
		magic:  string(data[0:4]),
		length: binary.BigEndian.Uint16(data[4:6]),
		chunks: make(map[uint16][]byte),
	}

	off := 6
	for off < len(data) {
		if off+6 > len(data) {
			t.Fatalf("truncated chunk header at offset %d", off)
		}
		// vendor   := binary.BigEndian.Uint16(data[off : off+2])
		cType := binary.BigEndian.Uint16(data[off+2 : off+4])
		cLen := int(binary.BigEndian.Uint16(data[off+4 : off+6]))
		if cLen < 6 || off+cLen > len(data) {
			t.Fatalf("invalid chunk length %d at offset %d", cLen, off)
		}
		value := data[off+6 : off+cLen]
		pf.chunks[cType] = value
		off += cLen
	}
	return pf
}

func makePacket() *core.OutputPacket {
	return &core.OutputPacket{
		TaskID:      "task-001",
		Timestamp:   time.Date(2024, 6, 1, 12, 0, 0, 500_000_000, time.UTC),
		SrcIP:       netip.MustParseAddr("192.168.1.10"),
		DstIP:       netip.MustParseAddr("10.0.0.1"),
		SrcPort:     5060,
		DstPort:     5060,
		Protocol:    17,
		PayloadType: "sip",
		RawPayload:  []byte("INVITE sip:bob@example.com SIP/2.0\r\n"),
		Labels: core.Labels{
			core.LabelSIPCallID:  "abc-123@host",
			core.LabelSIPFromURI: "sip:alice@example.com",
			core.LabelSIPToURI:   "sip:bob@example.com",
		},
	}
}

func TestEncode_MagicAndLength(t *testing.T) {
	frame, err := Encode(makePacket(), EncodeOptions{CaptureID: 42})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	pf := parseFrame(t, frame)

	if pf.magic != hepMagic {
		t.Errorf("magic = %q, want %q", pf.magic, hepMagic)
	}
	if int(pf.length) != len(frame) {
		t.Errorf("total length field = %d, actual frame len = %d", pf.length, len(frame))
	}
}

func TestEncode_IPFamilyAndProto(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{})
	pf := parseFrame(t, frame)

	// chunk 1: IPv4
	if got := pf.chunks[chunkIPFamily]; len(got) != 1 || got[0] != ipFamilyV4 {
		t.Errorf("chunk 1 (IP family) = %v, want [%d]", got, ipFamilyV4)
	}
	// chunk 2: UDP=17
	if got := pf.chunks[chunkIPProto]; len(got) != 1 || got[0] != 17 {
		t.Errorf("chunk 2 (IP proto) = %v, want [17]", got)
	}
}

func TestEncode_IPv4Addresses(t *testing.T) {
	pkt := makePacket()
	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	wantSrc := pkt.SrcIP.As4()
	if got := pf.chunks[chunkSrcIPv4]; string(got) != string(wantSrc[:]) {
		t.Errorf("chunk 3 (src IPv4) = %v, want %v", got, wantSrc)
	}
	wantDst := pkt.DstIP.As4()
	if got := pf.chunks[chunkDstIPv4]; string(got) != string(wantDst[:]) {
		t.Errorf("chunk 4 (dst IPv4) = %v, want %v", got, wantDst)
	}
}

func TestEncode_IPv6Addresses(t *testing.T) {
	pkt := makePacket()
	pkt.SrcIP = netip.MustParseAddr("2001:db8::1")
	pkt.DstIP = netip.MustParseAddr("2001:db8::2")

	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	if pf.chunks[chunkIPFamily][0] != ipFamilyV6 {
		t.Errorf("expected IPv6 family chunk")
	}
	wantSrc := pkt.SrcIP.As16()
	if string(pf.chunks[chunkSrcIPv6]) != string(wantSrc[:]) {
		t.Error("src IPv6 mismatch")
	}
}

func TestEncode_Ports(t *testing.T) {
	pkt := makePacket()
	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	srcPort := binary.BigEndian.Uint16(pf.chunks[chunkSrcPort])
	dstPort := binary.BigEndian.Uint16(pf.chunks[chunkDstPort])

	if srcPort != pkt.SrcPort {
		t.Errorf("src port = %d, want %d", srcPort, pkt.SrcPort)
	}
	if dstPort != pkt.DstPort {
		t.Errorf("dst port = %d, want %d", dstPort, pkt.DstPort)
	}
}

func TestEncode_Timestamp(t *testing.T) {
	pkt := makePacket()
	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	sec := binary.BigEndian.Uint32(pf.chunks[chunkTimeSec])
	usec := binary.BigEndian.Uint32(pf.chunks[chunkTimeUsec])

	if want := uint32(pkt.Timestamp.Unix()); sec != want {
		t.Errorf("timestamp sec = %d, want %d", sec, want)
	}
	if want := uint32(pkt.Timestamp.Nanosecond() / 1000); usec != want {
		t.Errorf("timestamp usec = %d, want %d", usec, want)
	}
}

func TestEncode_ProtoType_SIP(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{})
	pf := parseFrame(t, frame)

	if got := pf.chunks[chunkProtoType]; len(got) != 1 || got[0] != protoTypeSIP {
		t.Errorf("proto type = %v, want SIP (%d)", got, protoTypeSIP)
	}
}

func TestEncode_ProtoType_RTP(t *testing.T) {
	pkt := makePacket()
	pkt.PayloadType = "rtp"
	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	if got := pf.chunks[chunkProtoType]; len(got) != 1 || got[0] != protoTypeRTP {
		t.Errorf("proto type = %v, want RTP (%d)", got, protoTypeRTP)
	}
}

func TestEncode_ProtoType_RTCP(t *testing.T) {
	pkt := makePacket()
	pkt.PayloadType = "rtcp"
	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	if got := pf.chunks[chunkProtoType]; len(got) != 1 || got[0] != protoTypeRTCP {
		t.Errorf("proto type = %v, want RTCP (%d)", got, protoTypeRTCP)
	}
}

func TestEncode_CaptureID(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{CaptureID: 9999})
	pf := parseFrame(t, frame)

	id := binary.BigEndian.Uint32(pf.chunks[chunkCaptureID])
	if id != 9999 {
		t.Errorf("capture ID = %d, want 9999", id)
	}
}

func TestEncode_AuthKey_Present(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{AuthKey: "secret"})
	pf := parseFrame(t, frame)

	if got := string(pf.chunks[chunkAuthKey]); got != "secret" {
		t.Errorf("auth key = %q, want %q", got, "secret")
	}
}

func TestEncode_AuthKey_Absent(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{AuthKey: ""})
	pf := parseFrame(t, frame)

	if _, ok := pf.chunks[chunkAuthKey]; ok {
		t.Error("auth key chunk should be absent when AuthKey is empty")
	}
}

// TestEncode_Chunk19_NodeName verifies chunk 19 carries the configured node name.
func TestEncode_Chunk19_NodeName_Present(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{NodeName: "node-dc1-01"})
	pf := parseFrame(t, frame)

	if got := string(pf.chunks[chunkNodeName]); got != "node-dc1-01" {
		t.Errorf("chunk 19 (node name) = %q, want %q", got, "node-dc1-01")
	}
}

// TestEncode_Chunk19_NodeName_Absent verifies chunk 19 is omitted when NodeName is empty.
func TestEncode_Chunk19_NodeName_Absent(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{NodeName: ""})
	pf := parseFrame(t, frame)

	if _, ok := pf.chunks[chunkNodeName]; ok {
		t.Error("chunk 19 should be absent when NodeName is empty")
	}
}

func TestEncode_Payload(t *testing.T) {
	pkt := makePacket()
	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	if got := string(pf.chunks[chunkPayload]); got != string(pkt.RawPayload) {
		t.Errorf("payload = %q, want %q", got, pkt.RawPayload)
	}
}

func TestEncode_NoPayload(t *testing.T) {
	pkt := makePacket()
	pkt.RawPayload = nil
	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	if _, ok := pf.chunks[chunkPayload]; ok {
		t.Error("payload chunk should be absent when RawPayload is nil")
	}
}

func TestEncode_CorrID_SIPCallID(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{})
	pf := parseFrame(t, frame)

	if got := string(pf.chunks[chunkCorrID]); got != "abc-123@host" {
		t.Errorf("corr ID = %q, want %q", got, "abc-123@host")
	}
}

func TestEncode_CorrID_FallsBackToTaskID(t *testing.T) {
	pkt := makePacket()
	delete(pkt.Labels, core.LabelSIPCallID)
	delete(pkt.Labels, core.LabelRTPCallID)

	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	if got := string(pf.chunks[chunkCorrID]); got != pkt.TaskID {
		t.Errorf("corr ID = %q, want TaskID=%q", got, pkt.TaskID)
	}
}

// TestEncode_Chunk48_From verifies chunk 48 carries the SIP From-URI.
func TestEncode_Chunk48_From_SIPLabel(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{})
	pf := parseFrame(t, frame)

	if got := string(pf.chunks[chunkFrom]); got != "sip:alice@example.com" {
		t.Errorf("chunk 48 (from) = %q, want %q", got, "sip:alice@example.com")
	}
}

// TestEncode_Chunk49_To verifies chunk 49 carries the SIP To-URI.
func TestEncode_Chunk49_To_SIPLabel(t *testing.T) {
	frame, _ := Encode(makePacket(), EncodeOptions{})
	pf := parseFrame(t, frame)

	if got := string(pf.chunks[chunkTo]); got != "sip:bob@example.com" {
		t.Errorf("chunk 49 (to) = %q, want %q", got, "sip:bob@example.com")
	}
}

// TestEncode_Chunk48_From_FallbackToAddr verifies chunk 48 falls back to srcIP:port
// when the SIP From label is absent.
func TestEncode_Chunk48_From_Fallback(t *testing.T) {
	pkt := makePacket()
	delete(pkt.Labels, core.LabelSIPFromURI)

	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	want := "192.168.1.10:5060"
	if got := string(pf.chunks[chunkFrom]); got != want {
		t.Errorf("chunk 48 fallback = %q, want %q", got, want)
	}
}

// TestEncode_Chunk49_To_FallbackToAddr verifies chunk 49 falls back to dstIP:port.
func TestEncode_Chunk49_To_Fallback(t *testing.T) {
	pkt := makePacket()
	delete(pkt.Labels, core.LabelSIPToURI)

	frame, _ := Encode(pkt, EncodeOptions{})
	pf := parseFrame(t, frame)

	want := "10.0.0.1:5060"
	if got := string(pf.chunks[chunkTo]); got != want {
		t.Errorf("chunk 49 fallback = %q, want %q", got, want)
	}
}

func TestEncode_NilPacket(t *testing.T) {
	_, err := Encode(nil, EncodeOptions{})
	if err == nil {
		t.Error("expected error for nil packet")
	}
}

// ─── Reporter Init tests ───────────────────────────────────────────────────

func TestInit_MissingConfig(t *testing.T) {
	r := &HEPReporter{}
	if err := r.Init(nil); err == nil {
		t.Error("expected error for nil config")
	}
}

func TestInit_MissingServers(t *testing.T) {
	r := &HEPReporter{}
	if err := r.Init(map[string]any{}); err == nil {
		t.Error("expected error when servers is missing")
	}
}

func TestInit_EmptyServers(t *testing.T) {
	r := &HEPReporter{}
	err := r.Init(map[string]any{"servers": []any{}})
	if err == nil {
		t.Error("expected error when servers list is empty")
	}
}

func TestInit_ValidConfig(t *testing.T) {
	r := &HEPReporter{}
	err := r.Init(map[string]any{
		"servers":    []any{"127.0.0.1:9060", "127.0.0.2:9060"},
		"capture_id": float64(1001),
		"auth_key":   "tok",
		"node_name":  "edge-01",
	})
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}
	if r.config.CaptureID != 1001 {
		t.Errorf("CaptureID = %d, want 1001", r.config.CaptureID)
	}
	if r.config.AuthKey != "tok" {
		t.Errorf("AuthKey = %q, want %q", r.config.AuthKey, "tok")
	}
	if r.config.NodeName != "edge-01" {
		t.Errorf("NodeName = %q, want %q", r.config.NodeName, "edge-01")
	}
}

// ─── Reporter flow-routing tests ───────────────────────────────────────────

// TestSelectConn_SingleServer verifies it always returns the only connection.
func TestSelectConn_SingleServer(t *testing.T) {
	r := &HEPReporter{
		conns: []*net.UDPConn{nil}, // nil ok — we only test selection logic
	}
	pkt := makePacket()
	if got := r.selectConn(pkt); got != r.conns[0] {
		t.Error("single-server: expected conns[0]")
	}
}

// TestSelectConn_Stability verifies the same packet always maps to the same server.
func TestSelectConn_Stability(t *testing.T) {
	conns := make([]*net.UDPConn, 3)
	r := &HEPReporter{conns: conns}
	pkt := makePacket()

	first := r.selectConn(pkt)
	for i := 0; i < 20; i++ {
		if r.selectConn(pkt) != first {
			t.Fatal("selectConn returned different server for the same packet")
		}
	}
}

// TestSelectConn_Distribution verifies different flows go to different servers.
func TestSelectConn_Distribution(t *testing.T) {
	conns := make([]*net.UDPConn, 4)
	for i := range conns {
		conns[i] = &net.UDPConn{} // distinct pointers
	}
	r := &HEPReporter{conns: conns}

	seen := make(map[*net.UDPConn]bool)
	for srcPort := uint16(1024); srcPort < 1224; srcPort++ {
		pkt := makePacket()
		pkt.SrcPort = srcPort
		seen[r.selectConn(pkt)] = true
	}
	// With 200 distinct source ports we expect all 4 servers to be used.
	if len(seen) < len(conns) {
		t.Errorf("only %d/%d servers used — distribution problem", len(seen), len(conns))
	}
}

// ─── Reporter end-to-end UDP test ──────────────────────────────────────────

// TestReport_SendsHEPFrame starts a local UDP listener, runs the reporter and
// verifies at least one valid HEP frame is received.
func TestReport_SendsHEPFrame(t *testing.T) {
	// Start a local UDP echo server.
	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.ListenUDP("udp", laddr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	targetAddr := ln.LocalAddr().String()

	r := NewHEPReporter()
	if err := r.Init(map[string]any{
		"servers":    []any{targetAddr},
		"capture_id": float64(7777),
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	ctx := context.Background()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop(ctx) //nolint:errcheck

	pkt := makePacket()
	if err := r.Report(ctx, pkt); err != nil {
		t.Fatalf("Report: %v", err)
	}

	// Read from the UDP listener.
	buf := make([]byte, 4096)
	if err := ln.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	n, _, err := ln.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}
	frame := buf[:n]

	// Verify basic HEP structure.
	if string(frame[0:4]) != hepMagic {
		t.Errorf("magic = %q, want %q", frame[0:4], hepMagic)
	}
	if binary.BigEndian.Uint16(frame[4:6]) != uint16(n) {
		t.Errorf("length field %d != frame size %d", binary.BigEndian.Uint16(frame[4:6]), n)
	}

	// Verify chunk 48 (from) and chunk 49 (to) are present.
	pf := parseFrame(t, frame)
	if got := string(pf.chunks[chunkFrom]); got != "sip:alice@example.com" {
		t.Errorf("chunk 48 (from) = %q", got)
	}
	if got := string(pf.chunks[chunkTo]); got != "sip:bob@example.com" {
		t.Errorf("chunk 49 (to) = %q", got)
	}
}
