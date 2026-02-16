package decoder

import (
	"net/netip"
	"testing"
	"time"

	"firestige.xyz/otus/internal/core"
)

// Helper function to create a simple IPv4 UDP packet
func makeSimpleUDPPacket() []byte {
	packet := make([]byte, 42) // Ethernet + IPv4 + UDP headers

	// Ethernet header (14 bytes)
	// Dst MAC: 00:11:22:33:44:55
	packet[0], packet[1], packet[2] = 0x00, 0x11, 0x22
	packet[3], packet[4], packet[5] = 0x33, 0x44, 0x55
	// Src MAC: AA:BB:CC:DD:EE:FF
	packet[6], packet[7], packet[8] = 0xAA, 0xBB, 0xCC
	packet[9], packet[10], packet[11] = 0xDD, 0xEE, 0xFF
	// EtherType: IPv4 (0x0800)
	packet[12], packet[13] = 0x08, 0x00

	// IPv4 header (20 bytes)
	packet[14] = 0x45                   // Version 4, IHL 5
	packet[15] = 0x00                   // DSCP, ECN
	packet[16], packet[17] = 0x00, 0x1C // Total Length: 28 bytes
	packet[18], packet[19] = 0x12, 0x34 // Identification
	packet[20], packet[21] = 0x00, 0x00 // Flags, Fragment Offset
	packet[22] = 0x40                   // TTL: 64
	packet[23] = 0x11                   // Protocol: UDP (17)
	packet[24], packet[25] = 0x00, 0x00 // Checksum (not calculated)
	// Src IP: 192.168.1.1
	packet[26], packet[27], packet[28], packet[29] = 192, 168, 1, 1
	// Dst IP: 192.168.1.2
	packet[30], packet[31], packet[32], packet[33] = 192, 168, 1, 2

	// UDP header (8 bytes)
	packet[34], packet[35] = 0x13, 0x88 // Src Port: 5000
	packet[36], packet[37] = 0x13, 0x89 // Dst Port: 5001
	packet[38], packet[39] = 0x00, 0x08 // Length: 8 bytes
	packet[40], packet[41] = 0x00, 0x00 // Checksum (not calculated)

	return packet
}

func TestStandardDecoderDecode(t *testing.T) {
	decoder := NewStandardDecoder(Config{})

	raw := core.RawPacket{
		Data:       makeSimpleUDPPacket(),
		Timestamp:  time.Now(),
		CaptureLen: 42,
		OrigLen:    42,
	}

	decoded, err := decoder.Decode(raw)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify Ethernet header
	if decoded.Ethernet.EtherType != 0x0800 {
		t.Errorf("Expected EtherType 0x0800, got 0x%04x", decoded.Ethernet.EtherType)
	}
	expectedSrcMAC := [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	if decoded.Ethernet.SrcMAC != expectedSrcMAC {
		t.Errorf("Expected SrcMAC %v, got %v", expectedSrcMAC, decoded.Ethernet.SrcMAC)
	}

	// Verify IP header
	if decoded.IP.Version != 4 {
		t.Errorf("Expected IP version 4, got %d", decoded.IP.Version)
	}
	if decoded.IP.Protocol != 17 {
		t.Errorf("Expected protocol 17 (UDP), got %d", decoded.IP.Protocol)
	}
	expectedSrcIP := netip.MustParseAddr("192.168.1.1")
	if decoded.IP.SrcIP != expectedSrcIP {
		t.Errorf("Expected SrcIP %v, got %v", expectedSrcIP, decoded.IP.SrcIP)
	}
	expectedDstIP := netip.MustParseAddr("192.168.1.2")
	if decoded.IP.DstIP != expectedDstIP {
		t.Errorf("Expected DstIP %v, got %v", expectedDstIP, decoded.IP.DstIP)
	}

	// Verify Transport header
	if decoded.Transport.Protocol != 17 {
		t.Errorf("Expected transport protocol 17 (UDP), got %d", decoded.Transport.Protocol)
	}
	if decoded.Transport.SrcPort != 5000 {
		t.Errorf("Expected SrcPort 5000, got %d", decoded.Transport.SrcPort)
	}
	if decoded.Transport.DstPort != 5001 {
		t.Errorf("Expected DstPort 5001, got %d", decoded.Transport.DstPort)
	}
}

func TestStandardDecoderEmptyPacket(t *testing.T) {
	decoder := NewStandardDecoder(Config{})

	raw := core.RawPacket{
		Data:       []byte{},
		Timestamp:  time.Now(),
		CaptureLen: 0,
		OrigLen:    0,
	}

	_, err := decoder.Decode(raw)
	if err == nil {
		t.Error("Expected error for empty packet, got nil")
	}
}

func TestStandardDecoderTooShort(t *testing.T) {
	decoder := NewStandardDecoder(Config{})

	raw := core.RawPacket{
		Data:       []byte{0x01, 0x02, 0x03}, // Too short
		Timestamp:  time.Now(),
		CaptureLen: 3,
		OrigLen:    3,
	}

	_, err := decoder.Decode(raw)
	if err == nil {
		t.Error("Expected error for too short packet, got nil")
	}
}

func BenchmarkStandardDecoderDecode(b *testing.B) {
	decoder := NewStandardDecoder(Config{})
	packet := makeSimpleUDPPacket()

	raw := core.RawPacket{
		Data:       packet,
		Timestamp:  time.Now(),
		CaptureLen: uint32(len(packet)),
		OrigLen:    uint32(len(packet)),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := decoder.Decode(raw)
		if err != nil {
			b.Fatal(err)
		}
	}
}
