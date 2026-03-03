package decoder

import (
	"testing"
)

func TestDecodeUDP(t *testing.T) {
	// Minimal UDP header (8 bytes)
	data := []byte{
		0x13, 0x88, // Src Port: 5000
		0x13, 0x89, // Dst Port: 5001
		0x00, 0x0C, // Length: 12 bytes (8 header + 4 payload)
		0x00, 0x00, // Checksum
		0x01, 0x02, 0x03, 0x04, // Payload
	}

	transport, payload, err := decodeUDP(data)
	if err != nil {
		t.Fatalf("decodeUDP failed: %v", err)
	}

	// Check protocol
	if transport.Protocol != 17 {
		t.Errorf("Expected protocol 17, got %d", transport.Protocol)
	}

	// Check source port
	if transport.SrcPort != 5000 {
		t.Errorf("Expected SrcPort 5000, got %d", transport.SrcPort)
	}

	// Check destination port
	if transport.DstPort != 5001 {
		t.Errorf("Expected DstPort 5001, got %d", transport.DstPort)
	}

	// Check payload
	if len(payload) != 4 {
		t.Errorf("Expected payload length 4, got %d", len(payload))
	}
}

func TestDecodeUDPTooShort(t *testing.T) {
	data := []byte{0x13, 0x88, 0x13} // Too short

	_, _, err := decodeUDP(data)
	if err == nil {
		t.Error("Expected error for too short UDP packet, got nil")
	}
}

func TestDecodeTransportUDP(t *testing.T) {
	data := []byte{
		0x13, 0x88, 0x13, 0x89,
		0x00, 0x08, 0x00, 0x00,
	}

	transport, _, err := decodeTransport(data, 17)
	if err != nil {
		t.Fatalf("decodeTransport failed: %v", err)
	}

	if transport.Protocol != 17 {
		t.Errorf("Expected protocol 17, got %d", transport.Protocol)
	}
}

func TestDecodeTransportUnsupported(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}

	transport, payload, err := decodeTransport(data, 132) // SCTP
	if err != nil {
		t.Fatalf("decodeTransport failed: %v", err)
	}

	if transport.Protocol != 132 {
		t.Errorf("Expected protocol 132, got %d", transport.Protocol)
	}

	// For unsupported protocols, payload should be unchanged
	if len(payload) != len(data) {
		t.Errorf("Expected payload length %d, got %d", len(data), len(payload))
	}
}

func BenchmarkDecodeUDP(b *testing.B) {
	data := []byte{
		0x13, 0x88, 0x13, 0x89,
		0x00, 0x08, 0x00, 0x00,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := decodeUDP(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
