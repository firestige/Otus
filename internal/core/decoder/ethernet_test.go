package decoder

import (
	"testing"
)

func TestDecodeEthernetBasic(t *testing.T) {
	// Simple Ethernet frame: Dst MAC, Src MAC, EtherType
	data := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, // Dst MAC
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, // Src MAC
		0x08, 0x00, // EtherType: IPv4
		0x45, 0x00, // Payload (start of IP header)
	}

	eth, payload, err := decodeEthernet(data)
	if err != nil {
		t.Fatalf("decodeEthernet failed: %v", err)
	}

	// Check Dst MAC
	expectedDstMAC := [6]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	if eth.DstMAC != expectedDstMAC {
		t.Errorf("Expected DstMAC %v, got %v", expectedDstMAC, eth.DstMAC)
	}

	// Check Src MAC
	expectedSrcMAC := [6]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	if eth.SrcMAC != expectedSrcMAC {
		t.Errorf("Expected SrcMAC %v, got %v", expectedSrcMAC, eth.SrcMAC)
	}

	// Check EtherType
	if eth.EtherType != 0x0800 {
		t.Errorf("Expected EtherType 0x0800, got 0x%04x", eth.EtherType)
	}

	// Check payload
	if len(payload) != 2 {
		t.Errorf("Expected payload length 2, got %d", len(payload))
	}
}

func TestDecodeEthernetWithVLAN(t *testing.T) {
	// Ethernet frame with single VLAN tag
	data := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, // Dst MAC
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, // Src MAC
		0x81, 0x00, // EtherType: VLAN (0x8100)
		0x00, 0x0A, // VLAN TCI: VLAN ID 10
		0x08, 0x00, // Inner EtherType: IPv4
		0x45, 0x00, // Payload
	}

	eth, payload, err := decodeEthernet(data)
	if err != nil {
		t.Fatalf("decodeEthernet failed: %v", err)
	}

	// Check EtherType (should be inner EtherType)
	if eth.EtherType != 0x0800 {
		t.Errorf("Expected EtherType 0x0800, got 0x%04x", eth.EtherType)
	}

	// Check VLAN
	if len(eth.VLANs) != 1 {
		t.Fatalf("Expected 1 VLAN tag, got %d", len(eth.VLANs))
	}
	if eth.VLANs[0] != 10 {
		t.Errorf("Expected VLAN ID 10, got %d", eth.VLANs[0])
	}

	// Check payload
	if len(payload) != 2 {
		t.Errorf("Expected payload length 2, got %d", len(payload))
	}
}

func TestDecodeEthernetWithQinQ(t *testing.T) {
	// Ethernet frame with QinQ (double VLAN tags)
	data := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, // Dst MAC
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, // Src MAC
		0x88, 0xA8, // EtherType: QinQ (0x88A8)
		0x00, 0x14, // Outer VLAN: ID 20
		0x81, 0x00, // EtherType: VLAN (0x8100)
		0x00, 0x0A, // Inner VLAN: ID 10
		0x08, 0x00, // Inner EtherType: IPv4
		0x45, 0x00, // Payload
	}

	eth, _, err := decodeEthernet(data)
	if err != nil {
		t.Fatalf("decodeEthernet failed: %v", err)
	}

	// Check EtherType
	if eth.EtherType != 0x0800 {
		t.Errorf("Expected EtherType 0x0800, got 0x%04x", eth.EtherType)
	}

	// Check VLANs
	if len(eth.VLANs) != 2 {
		t.Fatalf("Expected 2 VLAN tags, got %d", len(eth.VLANs))
	}
	if eth.VLANs[0] != 20 {
		t.Errorf("Expected outer VLAN ID 20, got %d", eth.VLANs[0])
	}
	if eth.VLANs[1] != 10 {
		t.Errorf("Expected inner VLAN ID 10, got %d", eth.VLANs[1])
	}
}

func TestDecodeEthernetTooShort(t *testing.T) {
	data := []byte{0x00, 0x11, 0x22} // Too short

	_, _, err := decodeEthernet(data)
	if err == nil {
		t.Error("Expected error for too short packet, got nil")
	}
}

func BenchmarkDecodeEthernet(b *testing.B) {
	data := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55,
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
		0x08, 0x00,
		0x45, 0x00,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := decodeEthernet(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
