package decoder

import (
	"testing"
	"time"
)

func TestFragmentRateLimiter_NilWhenDisabled(t *testing.T) {
	l := NewFragmentRateLimiter(FragmentRateLimiterConfig{MaxFragsPerIP: 0})
	if l != nil {
		t.Error("expected nil when MaxFragsPerIP = 0")
	}
}

func TestFragmentRateLimiter_AllowsWithinLimit(t *testing.T) {
	l := NewFragmentRateLimiter(FragmentRateLimiterConfig{
		MaxFragsPerIP:   5,
		RateLimitWindow: 10 * time.Second,
	})

	srcIP := [4]byte{192, 168, 1, 1}
	now := time.Now()

	for i := 0; i < 5; i++ {
		if !l.Allow(srcIP, now) {
			t.Fatalf("fragment %d should be allowed (within limit)", i)
		}
	}
}

func TestFragmentRateLimiter_RejectsOverLimit(t *testing.T) {
	l := NewFragmentRateLimiter(FragmentRateLimiterConfig{
		MaxFragsPerIP:   3,
		RateLimitWindow: 10 * time.Second,
	})

	srcIP := [4]byte{10, 0, 0, 1}
	now := time.Now()

	for i := 0; i < 3; i++ {
		l.Allow(srcIP, now)
	}
	if l.Allow(srcIP, now) {
		t.Error("4th fragment should be rejected")
	}
	if l.Rejected() != 1 {
		t.Errorf("expected 1 rejected, got %d", l.Rejected())
	}
}

func TestFragmentRateLimiter_DifferentIPsIndependent(t *testing.T) {
	l := NewFragmentRateLimiter(FragmentRateLimiterConfig{
		MaxFragsPerIP:   2,
		RateLimitWindow: 10 * time.Second,
	})

	ip1 := [4]byte{1, 1, 1, 1}
	ip2 := [4]byte{2, 2, 2, 2}
	now := time.Now()

	l.Allow(ip1, now)
	l.Allow(ip1, now)
	if l.Allow(ip1, now) {
		t.Error("ip1's 3rd fragment should be rejected")
	}

	// ip2 should still be allowed (independent counter)
	if !l.Allow(ip2, now) {
		t.Error("ip2's 1st fragment should be allowed")
	}
}

func TestFragmentRateLimiter_WindowRotation(t *testing.T) {
	l := NewFragmentRateLimiter(FragmentRateLimiterConfig{
		MaxFragsPerIP:   2,
		RateLimitWindow: 1 * time.Second,
	})

	srcIP := [4]byte{10, 0, 0, 1}
	now := time.Now()

	// Exhaust the limit
	l.Allow(srcIP, now)
	l.Allow(srcIP, now)
	if l.Allow(srcIP, now) {
		t.Error("should be rejected before window rotation")
	}

	// Advance past the window
	later := now.Add(2 * time.Second)
	if !l.Allow(srcIP, later) {
		t.Error("should be allowed after window rotation")
	}
}

func TestFragmentRateLimiter_ActiveIPs(t *testing.T) {
	l := NewFragmentRateLimiter(FragmentRateLimiterConfig{
		MaxFragsPerIP:   100,
		RateLimitWindow: 10 * time.Second,
	})

	now := time.Now()
	l.Allow([4]byte{1, 0, 0, 1}, now)
	l.Allow([4]byte{2, 0, 0, 1}, now)
	l.Allow([4]byte{3, 0, 0, 1}, now)

	if got := l.ActiveIPs(); got != 3 {
		t.Errorf("expected 3 active IPs, got %d", got)
	}
}

func TestReassembler_RateLimitRejectsFragments(t *testing.T) {
	r := NewReassembler(ReassemblyConfig{
		MaxFragments:    100,
		MaxFragsPerIP:   2,
		RateLimitWindow: 60,
	})

	// Build 3 different fragments from the same source IP
	now := time.Now()
	for i := 0; i < 3; i++ {
		pkt := makeIPv4Fragment(
			[4]byte{192, 168, 1, 100}, // same src
			[4]byte{10, 0, 0, 1},
			uint16(1000), // same ID
			uint16(i)*10, // different offsets (in 8-byte units)
			i < 2,        // MF=1 for first two, MF=0 for last
			50,           // payload size
		)

		_, _, err := r.Process(pkt, now)
		if i < 2 && err != nil {
			t.Fatalf("fragment %d should be allowed, got error: %v", i, err)
		}
		if i == 2 && err == nil {
			t.Fatal("fragment 2 should be rejected by rate limiter")
		}
	}
}

func TestReassembler_RateLimitDisabledByDefault(t *testing.T) {
	r := NewReassembler(ReassemblyConfig{
		MaxFragments: 100,
		// MaxFragsPerIP not set → 0 → rate limiter disabled
	})

	// All fragments should be allowed
	now := time.Now()
	for i := 0; i < 50; i++ {
		pkt := makeIPv4Fragment(
			[4]byte{192, 168, 1, 1},
			[4]byte{10, 0, 0, 1},
			uint16(2000+i), // different IDs
			0,              // offset
			true,           // MF=1
			20,
		)
		_, _, err := r.Process(pkt, now)
		if err != nil {
			t.Fatalf("fragment %d should be allowed (rate limiting disabled), got: %v", i, err)
		}
	}
}

// makeIPv4Fragment builds a raw IPv4 fragment packet for testing.
func makeIPv4Fragment(srcIP, dstIP [4]byte, id, fragOffset8 uint16, moreFragments bool, payloadSize int) []byte {
	ihl := 20
	totalLen := ihl + payloadSize
	pkt := make([]byte, totalLen)

	// Version + IHL
	pkt[0] = 0x45 // IPv4, IHL=5 (20 bytes)

	// Total Length
	pkt[2] = byte(totalLen >> 8)
	pkt[3] = byte(totalLen)

	// Identification
	pkt[4] = byte(id >> 8)
	pkt[5] = byte(id)

	// Flags + Fragment Offset
	flagsOffset := fragOffset8
	if moreFragments {
		flagsOffset |= 0x2000 // MF flag
	}
	pkt[6] = byte(flagsOffset >> 8)
	pkt[7] = byte(flagsOffset)

	// Protocol (UDP)
	pkt[9] = 17

	// Source IP
	copy(pkt[12:16], srcIP[:])
	// Destination IP
	copy(pkt[16:20], dstIP[:])

	// Fill payload with sequential bytes
	for i := 0; i < payloadSize; i++ {
		pkt[ihl+i] = byte(i)
	}

	return pkt
}
