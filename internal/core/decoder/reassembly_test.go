package decoder

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// buildIPv4Fragment constructs a raw IPv4 packet with fragmentation fields set.
// srcIP/dstIP are 4-byte arrays, protocol is the IP protocol number.
// fragID is the identification field. fragOffset is in 8-byte units.
// moreFragments sets the MF flag. payload is the fragment payload data.
func buildIPv4Fragment(srcIP, dstIP [4]byte, protocol uint8, fragID uint16, fragOffset uint16, moreFragments bool, payload []byte) []byte {
	headerLen := 20
	totalLen := headerLen + len(payload)

	pkt := make([]byte, totalLen)

	// Version (4) + IHL (5 = 20 bytes)
	pkt[0] = 0x45
	// Total Length
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	// Identification
	binary.BigEndian.PutUint16(pkt[4:6], fragID)
	// Flags + Fragment Offset
	var flagsOffset uint16
	if moreFragments {
		flagsOffset |= 0x2000 // MF bit
	}
	flagsOffset |= fragOffset & 0x1FFF
	binary.BigEndian.PutUint16(pkt[6:8], flagsOffset)
	// TTL
	pkt[8] = 64
	// Protocol
	pkt[9] = protocol
	// Source IP
	copy(pkt[12:16], srcIP[:])
	// Destination IP
	copy(pkt[16:20], dstIP[:])
	// Payload
	copy(pkt[headerLen:], payload)

	return pkt
}

// buildNonFragmentedIPv4 constructs a non-fragmented IPv4 packet.
func buildNonFragmentedIPv4(payload []byte) []byte {
	return buildIPv4Fragment(
		[4]byte{10, 0, 0, 1}, [4]byte{10, 0, 0, 2},
		17, 0, 0, false, payload,
	)
}

func TestReassembler_NonFragment(t *testing.T) {
	r := NewReassembler(ReassemblyConfig{})
	now := time.Now()

	payload := []byte("hello, world")
	pkt := buildNonFragmentedIPv4(payload)

	result, complete, err := r.Process(pkt, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !complete {
		t.Fatal("non-fragmented packet should be complete")
	}
	if !bytes.Equal(result, payload) {
		t.Fatalf("expected payload %q, got %q", payload, result)
	}
}

func TestReassembler_TwoFragments(t *testing.T) {
	r := NewReassembler(ReassemblyConfig{})
	now := time.Now()

	src := [4]byte{192, 168, 1, 1}
	dst := [4]byte{192, 168, 1, 2}
	fragID := uint16(0x1234)
	proto := uint8(17) // UDP

	// Fragment 1: offset 0, MF=1, payload = bytes 0..79 (80 bytes)
	frag1Payload := make([]byte, 80)
	for i := range frag1Payload {
		frag1Payload[i] = byte(i)
	}
	pkt1 := buildIPv4Fragment(src, dst, proto, fragID, 0, true, frag1Payload)

	// Fragment 2: offset 10 (= 80 bytes), MF=0, payload = bytes 80..159 (80 bytes)
	frag2Payload := make([]byte, 80)
	for i := range frag2Payload {
		frag2Payload[i] = byte(80 + i)
	}
	pkt2 := buildIPv4Fragment(src, dst, proto, fragID, 10, false, frag2Payload) // offset 10 = 80 bytes

	// Process first fragment
	result, complete, err := r.Process(pkt1, now)
	if err != nil {
		t.Fatalf("fragment 1 error: %v", err)
	}
	if complete {
		t.Fatal("fragment 1 should not be complete")
	}
	if result != nil {
		t.Fatal("fragment 1 should return nil data")
	}

	// Process second fragment → should complete reassembly
	result, complete, err = r.Process(pkt2, now)
	if err != nil {
		t.Fatalf("fragment 2 error: %v", err)
	}
	if !complete {
		t.Fatal("fragment 2 should complete reassembly")
	}
	if len(result) != 160 {
		t.Fatalf("expected reassembled size 160, got %d", len(result))
	}

	// Verify payload content
	expected := make([]byte, 160)
	copy(expected[0:80], frag1Payload)
	copy(expected[80:160], frag2Payload)
	if !bytes.Equal(result, expected) {
		t.Fatal("reassembled payload mismatch")
	}
}

func TestReassembler_SIPFragment(t *testing.T) {
	// Simulate a ~3000 byte SIP INVITE split into 3 fragments
	// MTU 1500 → max IP payload ~1480 bytes per fragment (must be multiple of 8)
	r := NewReassembler(ReassemblyConfig{})
	now := time.Now()

	src := [4]byte{10, 1, 1, 100}
	dst := [4]byte{10, 1, 1, 200}
	fragID := uint16(0xABCD)
	proto := uint8(17) // UDP

	// Build a 3000-byte "SIP INVITE" payload
	totalPayload := make([]byte, 3000)
	for i := range totalPayload {
		totalPayload[i] = byte(i % 256)
	}

	// Fragment 1: offset=0, len=1480, MF=1
	frag1 := buildIPv4Fragment(src, dst, proto, fragID, 0, true, totalPayload[0:1480])
	// Fragment 2: offset=185 (185*8=1480), len=1480, MF=1
	frag2 := buildIPv4Fragment(src, dst, proto, fragID, 185, true, totalPayload[1480:2960])
	// Fragment 3: offset=370 (370*8=2960), len=40, MF=0
	frag3 := buildIPv4Fragment(src, dst, proto, fragID, 370, false, totalPayload[2960:3000])

	// Send in order
	_, complete, err := r.Process(frag1, now)
	if err != nil {
		t.Fatalf("frag1: %v", err)
	}
	if complete {
		t.Fatal("frag1 should not complete")
	}

	_, complete, err = r.Process(frag2, now)
	if err != nil {
		t.Fatalf("frag2: %v", err)
	}
	if complete {
		t.Fatal("frag2 should not complete")
	}

	result, complete, err := r.Process(frag3, now)
	if err != nil {
		t.Fatalf("frag3: %v", err)
	}
	if !complete {
		t.Fatal("frag3 should complete reassembly")
	}
	if !bytes.Equal(result, totalPayload) {
		t.Fatalf("reassembled payload mismatch: got %d bytes, want %d bytes", len(result), len(totalPayload))
	}
}

func TestReassembler_OutOfOrder(t *testing.T) {
	// Fragments arrive in reverse order
	r := NewReassembler(ReassemblyConfig{})
	now := time.Now()

	src := [4]byte{10, 0, 0, 1}
	dst := [4]byte{10, 0, 0, 2}
	fragID := uint16(0x5678)
	proto := uint8(17)

	payload := make([]byte, 240)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	// 3 fragments of 80 bytes each, sent in reverse order
	frag3 := buildIPv4Fragment(src, dst, proto, fragID, 20, false, payload[160:240]) // offset 20*8=160
	frag2 := buildIPv4Fragment(src, dst, proto, fragID, 10, true, payload[80:160])   // offset 10*8=80
	frag1 := buildIPv4Fragment(src, dst, proto, fragID, 0, true, payload[0:80])      // offset 0

	// Process last fragment first (MF=0)
	_, complete, err := r.Process(frag3, now)
	if err != nil {
		t.Fatalf("frag3: %v", err)
	}
	if complete {
		t.Fatal("frag3 alone should not complete")
	}

	// Middle fragment
	_, complete, err = r.Process(frag2, now)
	if err != nil {
		t.Fatalf("frag2: %v", err)
	}
	if complete {
		t.Fatal("frag2 should not complete")
	}

	// First fragment → should complete
	result, complete, err := r.Process(frag1, now)
	if err != nil {
		t.Fatalf("frag1: %v", err)
	}
	if !complete {
		t.Fatal("frag1 should complete reassembly")
	}
	if !bytes.Equal(result, payload) {
		t.Fatal("reassembled payload mismatch")
	}
}

func TestReassembler_OverlappingFragments(t *testing.T) {
	// BSD-Right: earlier-arrived fragment data takes priority on overlap
	r := NewReassembler(ReassemblyConfig{})
	now := time.Now()

	src := [4]byte{10, 0, 0, 1}
	dst := [4]byte{10, 0, 0, 2}
	fragID := uint16(0x9999)
	proto := uint8(17)

	// Fragment 1: offset=0, len=80, MF=1, data=[0xAA]*80
	frag1Payload := bytes.Repeat([]byte{0xAA}, 80)
	frag1 := buildIPv4Fragment(src, dst, proto, fragID, 0, true, frag1Payload)

	// Fragment 2: offset=5 (40 bytes), len=80, MF=0, data=[0xBB]*80
	// Overlaps with frag1 at bytes 40-79.
	// BSD-Right: frag1 data at 40-79 should be preserved, frag2 only contributes 80-119
	frag2Payload := bytes.Repeat([]byte{0xBB}, 80)
	frag2 := buildIPv4Fragment(src, dst, proto, fragID, 5, false, frag2Payload) // 5*8=40

	_, complete, err := r.Process(frag1, now)
	if err != nil {
		t.Fatalf("frag1: %v", err)
	}
	if complete {
		t.Fatal("frag1 should not complete")
	}

	result, complete, err := r.Process(frag2, now)
	if err != nil {
		t.Fatalf("frag2: %v", err)
	}
	if !complete {
		t.Fatal("fragments should be complete")
	}

	// Expected: bytes 0-79 = 0xAA (from frag1), bytes 80-119 = 0xBB (from frag2 non-overlapping part)
	if len(result) != 120 {
		t.Fatalf("expected 120 bytes, got %d", len(result))
	}
	for i := 0; i < 80; i++ {
		if result[i] != 0xAA {
			t.Fatalf("byte %d: expected 0xAA (from frag1), got 0x%02X", i, result[i])
		}
	}
	for i := 80; i < 120; i++ {
		if result[i] != 0xBB {
			t.Fatalf("byte %d: expected 0xBB (from frag2), got 0x%02X", i, result[i])
		}
	}
}

func TestReassembler_DuplicateFragment(t *testing.T) {
	// Duplicate fragment should be discarded (fully overlapped)
	r := NewReassembler(ReassemblyConfig{})
	now := time.Now()

	src := [4]byte{10, 0, 0, 1}
	dst := [4]byte{10, 0, 0, 2}
	fragID := uint16(0x1111)
	proto := uint8(17)

	frag1 := buildIPv4Fragment(src, dst, proto, fragID, 0, true, bytes.Repeat([]byte{0xAA}, 80))
	frag1Dup := buildIPv4Fragment(src, dst, proto, fragID, 0, true, bytes.Repeat([]byte{0xBB}, 80))
	frag2 := buildIPv4Fragment(src, dst, proto, fragID, 10, false, bytes.Repeat([]byte{0xCC}, 80))

	r.Process(frag1, now)
	r.Process(frag1Dup, now) // Should be discarded

	result, complete, err := r.Process(frag2, now)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !complete {
		t.Fatal("should be complete")
	}

	// Bytes 0-79 should be 0xAA (first arrived), not 0xBB
	for i := 0; i < 80; i++ {
		if result[i] != 0xAA {
			t.Fatalf("byte %d: expected 0xAA, got 0x%02X — duplicate was not discarded", i, result[i])
		}
	}
}

func TestReassembler_SecurityChecks(t *testing.T) {
	r := NewReassembler(ReassemblyConfig{})
	now := time.Now()

	src := [4]byte{10, 0, 0, 1}
	dst := [4]byte{10, 0, 0, 2}

	t.Run("FragmentOffsetTooLarge", func(t *testing.T) {
		// fragOffset > ipv4MaxFragOffset (8183)
		pkt := buildIPv4Fragment(src, dst, 17, 0x1234, 8184, true, []byte{0x01})
		_, _, err := r.Process(pkt, now)
		if err == nil {
			t.Fatal("expected error for oversized fragment offset")
		}
	})

	t.Run("FragmentExceedsMaxIPSize", func(t *testing.T) {
		// offset * 8 + size > 65535
		// offset = 8183 * 8 = 65464 bytes, payload = 80 bytes → 65544 > 65535
		pkt := buildIPv4Fragment(src, dst, 17, 0x1234, 8183, true, make([]byte, 80))
		_, _, err := r.Process(pkt, now)
		if err == nil {
			t.Fatal("expected error for fragment exceeding max IP size")
		}
	})

	t.Run("PacketTooShort", func(t *testing.T) {
		// Less than 20 bytes
		_, _, err := r.Process([]byte{0x45, 0x00}, now)
		if err == nil {
			t.Fatal("expected error for too-short packet")
		}
	})

	t.Run("InvalidIHL", func(t *testing.T) {
		// IHL = 1 (4 bytes, less than minimum 20)
		pkt := make([]byte, 20)
		pkt[0] = 0x41 // version=4, IHL=1
		_, _, err := r.Process(pkt, now)
		if err == nil {
			t.Fatal("expected error for invalid IHL")
		}
	})
}

func TestReassembler_MaxFragListLen(t *testing.T) {
	// Fragment count limit should trigger eviction and error
	r := NewReassembler(ReassemblyConfig{MaxFragments: 3})
	now := time.Now()

	src := [4]byte{10, 0, 0, 1}
	dst := [4]byte{10, 0, 0, 2}
	fragID := uint16(0x2222)

	// Send 3 fragments (fill the limit)
	for i := 0; i < 3; i++ {
		offset := uint16(i * 1) // each 8 bytes, offset in 8-byte units
		pkt := buildIPv4Fragment(src, dst, 17, fragID, offset, true, make([]byte, 8))
		_, _, err := r.Process(pkt, now)
		if err != nil {
			t.Fatalf("fragment %d: unexpected error: %v", i, err)
		}
	}

	// 4th fragment should exceed limit
	pkt := buildIPv4Fragment(src, dst, 17, fragID, 3, false, make([]byte, 8))
	_, _, err := r.Process(pkt, now)
	if err == nil {
		t.Fatal("expected error when exceeding MaxFragments limit")
	}
}

func TestReassembler_DifferentFlows(t *testing.T) {
	// Fragments from different flows should be tracked independently
	r := NewReassembler(ReassemblyConfig{})
	now := time.Now()

	src1 := [4]byte{10, 0, 0, 1}
	dst1 := [4]byte{10, 0, 0, 2}
	src2 := [4]byte{10, 0, 0, 3}
	dst2 := [4]byte{10, 0, 0, 4}

	payload1 := bytes.Repeat([]byte{0x11}, 80)
	payload2 := bytes.Repeat([]byte{0x22}, 80)

	// Flow 1 fragment 1
	r.Process(buildIPv4Fragment(src1, dst1, 17, 0x1111, 0, true, payload1), now)
	// Flow 2 fragment 1
	r.Process(buildIPv4Fragment(src2, dst2, 17, 0x2222, 0, true, payload2), now)

	// Flow 1 fragment 2
	result1, complete1, err := r.Process(
		buildIPv4Fragment(src1, dst1, 17, 0x1111, 10, false, bytes.Repeat([]byte{0x33}, 80)), now)
	if err != nil {
		t.Fatalf("flow1 frag2: %v", err)
	}
	if !complete1 {
		t.Fatal("flow1 should be complete")
	}
	if result1[0] != 0x11 {
		t.Fatal("flow1 data corrupted")
	}

	// Flow 2 fragment 2
	result2, complete2, err := r.Process(
		buildIPv4Fragment(src2, dst2, 17, 0x2222, 10, false, bytes.Repeat([]byte{0x44}, 80)), now)
	if err != nil {
		t.Fatalf("flow2 frag2: %v", err)
	}
	if !complete2 {
		t.Fatal("flow2 should be complete")
	}
	if result2[0] != 0x22 {
		t.Fatal("flow2 data corrupted")
	}
}

func TestReassembler_FlowEvictionAfterComplete(t *testing.T) {
	// After successful reassembly, the flow should be removed from the map
	r := NewReassembler(ReassemblyConfig{})
	now := time.Now()

	src := [4]byte{10, 0, 0, 1}
	dst := [4]byte{10, 0, 0, 2}
	fragID := uint16(0x3333)

	r.Process(buildIPv4Fragment(src, dst, 17, fragID, 0, true, make([]byte, 80)), now)
	r.Process(buildIPv4Fragment(src, dst, 17, fragID, 10, false, make([]byte, 80)), now)

	// Flow should be cleaned up
	r.mu.Lock()
	key := fragmentKey{protocol: 17, id: fragID}
	copy(key.srcIP[:], src[:])
	copy(key.dstIP[:], dst[:])
	_, exists := r.flows[key]
	r.mu.Unlock()

	if exists {
		t.Fatal("flow should be evicted after successful reassembly")
	}
}
