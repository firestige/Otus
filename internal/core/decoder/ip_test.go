package decoder

import (
	"net/netip"
	"testing"
)

func TestDecodeIPv4Basic(t *testing.T) {
	// Minimal IPv4 header (20 bytes)
	data := []byte{
		0x45,                   // Version 4, IHL 5
		0x00,                   // DSCP, ECN
		0x00, 0x1C,             // Total Length: 28 bytes
		0x12, 0x34,             // Identification
		0x00, 0x00,             // Flags, Fragment Offset
		0x40,                   // TTL: 64
		0x11,                   // Protocol: UDP (17)
		0x00, 0x00,             // Checksum
		192, 168, 1, 1,         // Src IP
		192, 168, 1, 2,         // Dst IP
		0x01, 0x02, 0x03, 0x04, // Payload
	}

	ip, payload, err := decodeIPv4(data)
	if err != nil {
		t.Fatalf("decodeIPv4 failed: %v", err)
	}

	// Check version
	if ip.Version != 4 {
		t.Errorf("Expected version 4, got %d", ip.Version)
	}

	// Check protocol
	if ip.Protocol != 17 {
		t.Errorf("Expected protocol 17, got %d", ip.Protocol)
	}

	// Check TTL
	if ip.TTL != 64 {
		t.Errorf("Expected TTL 64, got %d", ip.TTL)
	}

	// Check total length
	if ip.TotalLen != 28 {
		t.Errorf("Expected TotalLen 28, got %d", ip.TotalLen)
	}

	// Check source IP
	expectedSrcIP := netip.MustParseAddr("192.168.1.1")
	if ip.SrcIP != expectedSrcIP {
		t.Errorf("Expected SrcIP %v, got %v", expectedSrcIP, ip.SrcIP)
	}

	// Check destination IP
	expectedDstIP := netip.MustParseAddr("192.168.1.2")
	if ip.DstIP != expectedDstIP {
		t.Errorf("Expected DstIP %v, got %v", expectedDstIP, ip.DstIP)
	}

	// Check payload
	if len(payload) != 4 {
		t.Errorf("Expected payload length 4, got %d", len(payload))
	}
}

func TestDecodeIPv6Basic(t *testing.T) {
	// Minimal IPv6 header (40 bytes)
	data := make([]byte, 40+4) // Header + payload

	// Version (6), Traffic Class, Flow Label
	data[0] = 0x60 // Version 6

	// Payload Length
	data[4], data[5] = 0x00, 0x04 // 4 bytes

	// Next Header (Protocol)
	data[6] = 17 // UDP

	// Hop Limit (TTL)
	data[7] = 64

	// Source IP: 2001:db8::1
	copy(data[8:24], []byte{
		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	})

	// Destination IP: 2001:db8::2
	copy(data[24:40], []byte{
		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
	})

	// Payload
	data[40], data[41], data[42], data[43] = 0x01, 0x02, 0x03, 0x04

	ip, payload, err := decodeIPv6(data)
	if err != nil {
		t.Fatalf("decodeIPv6 failed: %v", err)
	}

	// Check version
	if ip.Version != 6 {
		t.Errorf("Expected version 6, got %d", ip.Version)
	}

	// Check protocol
	if ip.Protocol != 17 {
		t.Errorf("Expected protocol 17, got %d", ip.Protocol)
	}

	// Check TTL
	if ip.TTL != 64 {
		t.Errorf("Expected TTL 64, got %d", ip.TTL)
	}

	// Check source IP
	expectedSrcIP := netip.MustParseAddr("2001:db8::1")
	if ip.SrcIP != expectedSrcIP {
		t.Errorf("Expected SrcIP %v, got %v", expectedSrcIP, ip.SrcIP)
	}

	// Check destination IP
	expectedDstIP := netip.MustParseAddr("2001:db8::2")
	if ip.DstIP != expectedDstIP {
		t.Errorf("Expected DstIP %v, got %v", expectedDstIP, ip.DstIP)
	}

	// Check payload
	if len(payload) != 4 {
		t.Errorf("Expected payload length 4, got %d", len(payload))
	}
}

func TestDecodeIPTooShort(t *testing.T) {
	data := []byte{0x45, 0x00, 0x00} // Too short

	_, _, err := decodeIP(data)
	if err == nil {
		t.Error("Expected error for too short packet, got nil")
	}
}

func TestDecodeIPUnsupportedVersion(t *testing.T) {
	data := make([]byte, 20)
	data[0] = 0x70 // Version 7 (invalid)

	_, _, err := decodeIP(data)
	if err == nil {
		t.Error("Expected error for unsupported IP version, got nil")
	}
}

func TestIsIPFragmentTrue(t *testing.T) {
	// IPv4 packet with MF flag set
	data := []byte{
		0x45,                   // Version 4, IHL 5
		0x00,                   // DSCP, ECN
		0x00, 0x1C,             // Total Length: 28 bytes
		0x12, 0x34,             // Identification
		0x20, 0x00,             // Flags (MF=1), Fragment Offset = 0
		0x40,                   // TTL: 64
		0x11,                   // Protocol: UDP (17)
		0x00, 0x00,             // Checksum
		192, 168, 1, 1,         // Src IP
		192, 168, 1, 2,         // Dst IP
	}

	if !isIPFragment(data, 4) {
		t.Error("Expected fragment with MF flag, got non-fragment")
	}
}

func TestIsIPFragmentFalse(t *testing.T) {
	// IPv4 packet without fragmentation
	data := []byte{
		0x45,                   // Version 4, IHL 5
		0x00,                   // DSCP, ECN
		0x00, 0x1C,             // Total Length: 28 bytes
		0x12, 0x34,             // Identification
		0x00, 0x00,             // Flags, Fragment Offset = 0
		0x40,                   // TTL: 64
		0x11,                   // Protocol: UDP (17)
		0x00, 0x00,             // Checksum
		192, 168, 1, 1,         // Src IP
		192, 168, 1, 2,         // Dst IP
	}

	if isIPFragment(data, 4) {
		t.Error("Expected non-fragment, got fragment")
	}
}

func BenchmarkDecodeIPv4(b *testing.B) {
	data := []byte{
		0x45, 0x00, 0x00, 0x1C,
		0x12, 0x34, 0x00, 0x00,
		0x40, 0x11, 0x00, 0x00,
		192, 168, 1, 1,
		192, 168, 1, 2,
		0x01, 0x02, 0x03, 0x04,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := decodeIPv4(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeIPv6(b *testing.B) {
	data := make([]byte, 44)
	data[0] = 0x60
	data[6] = 17
	data[7] = 64
	copy(data[8:24], []byte{
		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	})
	copy(data[24:40], []byte{
		0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := decodeIPv6(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
