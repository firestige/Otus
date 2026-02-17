// Package decoder implements protocol decoding.
package decoder

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"firestige.xyz/otus/internal/metrics"
)

// Reassembly constants ported from BSD-Right algorithm (RFC 791).
const (
	ipv4MinFragSize    = 1     // Minimum valid fragment payload size
	ipv4MaxSize        = 65535 // Maximum IPv4 datagram size
	ipv4MaxFragOffset  = 8183  // Maximum valid fragment offset (in 8-byte units)
	ipv4MaxFragListLen = 8192  // Maximum fragments per flow before eviction
)

// ReassemblyConfig contains configuration for IP reassembly.
type ReassemblyConfig struct {
	MaxFragments      int // Maximum fragments per flow (default 100)
	MaxReassembleSize int // Maximum reassembled packet size (default 65535)
	Timeout           int // Timeout in seconds (default 60)
	MaxFragsPerIP     int // Per-source-IP fragment rate limit per window (0 = disabled)
	RateLimitWindow   int // Rate limit window in seconds (default 10)
}

// fragmentKey uniquely identifies a fragmented IPv4 datagram.
// Uses fixed-size arrays to avoid string allocation in the hot path.
type fragmentKey struct {
	srcIP    [4]byte
	dstIP    [4]byte
	protocol uint8
	id       uint16
}

// fragment represents a single IP fragment's payload and position.
type fragment struct {
	offset  uint16 // Fragment offset in bytes (fragOffset * 8)
	length  uint16 // Payload length in bytes
	payload []byte // Fragment payload (copy of original data)
}

// fragmentList implements BSD-Right ordered insertion for IP fragment reassembly.
// Fragments are maintained in sorted order by offset. When a new fragment
// overlaps with existing ones, the existing (earlier-arrived) data is preserved
// and the new fragment's overlapping portion is trimmed (BSD-Right policy).
type fragmentList struct {
	mu            sync.Mutex
	list          list.List // list of *fragment, sorted by offset ascending
	highest       uint16    // highest byte position seen = max(offset + fragLen)
	current       uint16    // total unique bytes accumulated
	finalReceived bool      // true when the last fragment (MF=0) is received
	lastSeen      time.Time // timestamp of last fragment for timeout cleanup
}

// Reassembler handles IPv4 fragment reassembly using BSD-Right algorithm.
type Reassembler struct {
	mu          sync.Mutex
	flows       map[fragmentKey]*fragmentList
	config      ReassemblyConfig
	rateLimiter *FragmentRateLimiter // nil if rate limiting disabled
}

// NewReassembler creates a new IP fragment reassembler.
func NewReassembler(cfg ReassemblyConfig) *Reassembler {
	if cfg.MaxFragments <= 0 {
		cfg.MaxFragments = 100
	}
	if cfg.MaxReassembleSize <= 0 {
		cfg.MaxReassembleSize = ipv4MaxSize
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60
	}

	r := &Reassembler{
		flows:  make(map[fragmentKey]*fragmentList),
		config: cfg,
		rateLimiter: NewFragmentRateLimiter(FragmentRateLimiterConfig{
			MaxFragsPerIP:   cfg.MaxFragsPerIP,
			RateLimitWindow: time.Duration(cfg.RateLimitWindow) * time.Second,
		}),
	}

	// Start cleanup goroutine for expired fragments
	go r.cleanup()

	return r
}

// Process processes raw IPv4 packet bytes (including IP header).
// Returns:
//   - Non-fragmented packet: (payload, true, nil) — fast path, no copy
//   - Fragment not yet complete: (nil, false, nil) — waiting for more fragments
//   - Fragment reassembled: (reassembledPayload, true, nil) — complete datagram
//   - Error: (nil, false, err) — security check failed or limits exceeded
func (r *Reassembler) Process(ipData []byte, timestamp time.Time) ([]byte, bool, error) {
	if len(ipData) < 20 {
		return nil, false, fmt.Errorf("IP packet too short: %d bytes", len(ipData))
	}

	// Parse fragmentation-relevant fields directly from raw bytes.
	// IPv4 header layout:
	//   byte 0:     version(4) + IHL(4)
	//   bytes 2-3:  Total Length
	//   bytes 4-5:  Identification (fragment ID)
	//   bytes 6-7:  Flags(3) + Fragment Offset(13)
	//   byte 9:     Protocol
	//   bytes 12-15: Source IP
	//   bytes 16-19: Destination IP

	ihl := int(ipData[0]&0x0F) * 4
	if ihl < 20 || len(ipData) < ihl {
		return nil, false, fmt.Errorf("invalid IHL: %d", ihl)
	}

	totalLen := int(binary.BigEndian.Uint16(ipData[2:4]))
	if totalLen < ihl {
		totalLen = len(ipData) // Clamp to actual data length if bogus
	}
	if totalLen > len(ipData) {
		totalLen = len(ipData)
	}

	id := binary.BigEndian.Uint16(ipData[4:6])
	flagsOffset := binary.BigEndian.Uint16(ipData[6:8])
	moreFragments := (flagsOffset & 0x2000) != 0 // MF flag (bit 13)
	fragOffset := flagsOffset & 0x1FFF           // Fragment offset in 8-byte units

	// Fast path: non-fragmented packet (MF=0 and offset=0)
	if !moreFragments && fragOffset == 0 {
		return ipData[ihl:totalLen], true, nil
	}

	// Calculate actual byte offset and payload size
	byteOffset := fragOffset * 8
	fragPayloadLen := uint16(totalLen - ihl)

	// Security checks (ported from reference implementation)
	if err := r.securityChecks(fragPayloadLen, fragOffset); err != nil {
		return nil, false, err
	}

	// Per-source-IP rate limiting (DoS protection)
	var srcIPKey [4]byte
	copy(srcIPKey[:], ipData[12:16])
	if r.rateLimiter != nil && !r.rateLimiter.Allow(srcIPKey, timestamp) {
		return nil, false, fmt.Errorf("fragment rate limit exceeded for source IP %d.%d.%d.%d",
			srcIPKey[0], srcIPKey[1], srcIPKey[2], srcIPKey[3])
	}

	// Build fragment key from raw bytes
	key := fragmentKey{
		protocol: ipData[9],
		id:       id,
	}
	copy(key.srcIP[:], ipData[12:16])
	copy(key.dstIP[:], ipData[16:20])

	// Get or create fragment list for this flow
	r.mu.Lock()
	fl, exists := r.flows[key]
	if !exists {
		fl = &fragmentList{}
		r.flows[key] = fl
		metrics.ReassemblyActiveFragments.Inc()
	}
	r.mu.Unlock()

	// Copy fragment payload (the original buffer may be reused by the capture ring)
	payload := make([]byte, fragPayloadLen)
	copy(payload, ipData[ihl:totalLen])

	fl.mu.Lock()
	defer fl.mu.Unlock()

	// Check fragment list length limit
	if fl.list.Len() >= ipv4MaxFragListLen {
		fl.mu.Unlock()
		r.evictFlow(key)
		fl.mu.Lock()
		return nil, false, fmt.Errorf("fragment list exceeded max size %d", ipv4MaxFragListLen)
	}

	// Check per-flow fragment count limit from config
	if fl.list.Len() >= r.config.MaxFragments {
		fl.mu.Unlock()
		r.evictFlow(key)
		fl.mu.Lock()
		return nil, false, fmt.Errorf("fragment count exceeded limit %d", r.config.MaxFragments)
	}

	fl.lastSeen = timestamp

	// Record if this is the last fragment
	if !moreFragments {
		fl.finalReceived = true
		endPos := byteOffset + fragPayloadLen
		if endPos > fl.highest {
			fl.highest = endPos
		}
	}

	// BSD-Right ordered insert
	frag := &fragment{
		offset:  byteOffset,
		length:  fragPayloadLen,
		payload: payload,
	}
	r.insertBSDRight(fl, frag)

	// Check if reassembly is complete
	if fl.finalReceived && fl.current >= fl.highest {
		result, err := r.build(fl)
		fl.mu.Unlock()
		r.evictFlow(key)
		fl.mu.Lock()
		if err != nil {
			return nil, false, err
		}
		return result, true, nil
	}

	return nil, false, nil
}

// securityChecks validates fragment parameters to prevent attacks.
func (r *Reassembler) securityChecks(fragSize, fragOffset uint16) error {
	if fragSize < ipv4MinFragSize {
		return fmt.Errorf("fragment too small: %d bytes", fragSize)
	}
	if fragOffset > ipv4MaxFragOffset {
		return fmt.Errorf("fragment offset too large: %d", fragOffset)
	}
	// Check reconstructed size doesn't exceed IPv4 max
	endPos := uint32(fragOffset)*8 + uint32(fragSize)
	if endPos > ipv4MaxSize {
		return fmt.Errorf("fragment would exceed max IP size: offset=%d size=%d end=%d",
			fragOffset*8, fragSize, endPos)
	}
	return nil
}

// insertBSDRight inserts a fragment into the ordered list using BSD-Right policy.
// Existing fragments take priority over new ones on overlap (keep earlier data).
// Must be called with fl.mu held.
func (r *Reassembler) insertBSDRight(fl *fragmentList, frag *fragment) {
	fragEnd := frag.offset + frag.length

	// Update highest byte position for non-final fragments
	if fragEnd > fl.highest && !fl.finalReceived {
		fl.highest = fragEnd
	}

	// Find insertion point: first element with offset >= frag.offset
	var insertBefore *list.Element
	for e := fl.list.Front(); e != nil; e = e.Next() {
		existing := e.Value.(*fragment)
		if existing.offset >= frag.offset {
			insertBefore = e
			break
		}
	}

	// Determine effective start after trimming overlap with previous fragment
	startAt := frag.offset
	if insertBefore != nil {
		if prev := insertBefore.Prev(); prev != nil {
			prevFrag := prev.Value.(*fragment)
			prevEnd := prevFrag.offset + prevFrag.length
			if prevEnd > startAt {
				startAt = prevEnd
			}
		}
	} else if fl.list.Len() > 0 {
		// insertBefore is nil → goes at end; check overlap with last element
		lastFrag := fl.list.Back().Value.(*fragment)
		lastEnd := lastFrag.offset + lastFrag.length
		if lastEnd > startAt {
			startAt = lastEnd
		}
	}

	// Determine effective end after trimming overlap with next fragment
	endAt := fragEnd
	if insertBefore != nil {
		nextFrag := insertBefore.Value.(*fragment)
		if nextFrag.offset < endAt {
			endAt = nextFrag.offset
		}
	}

	// After trimming, check if anything remains
	if startAt >= endAt {
		return // Fully overlapped by existing fragments — discard
	}

	// Trim the payload
	trimmedOffset := startAt - frag.offset
	trimmedEnd := endAt - frag.offset
	trimmedFrag := &fragment{
		offset:  startAt,
		length:  endAt - startAt,
		payload: frag.payload[trimmedOffset:trimmedEnd],
	}

	// Insert into list at correct position
	if insertBefore != nil {
		fl.list.InsertBefore(trimmedFrag, insertBefore)
	} else {
		fl.list.PushBack(trimmedFrag)
	}

	// Update current byte count
	fl.current += trimmedFrag.length
}

// build reassembles all fragments into a contiguous payload.
// Must be called with fl.mu held.
func (r *Reassembler) build(fl *fragmentList) ([]byte, error) {
	totalSize := int(fl.highest)
	if totalSize > r.config.MaxReassembleSize {
		return nil, fmt.Errorf("reassembled size %d exceeds limit %d", totalSize, r.config.MaxReassembleSize)
	}

	result := make([]byte, totalSize)
	for e := fl.list.Front(); e != nil; e = e.Next() {
		frag := e.Value.(*fragment)
		copy(result[frag.offset:frag.offset+frag.length], frag.payload)
	}

	return result, nil
}

// evictFlow removes a flow from the map and decrements the metric.
func (r *Reassembler) evictFlow(key fragmentKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.flows[key]; exists {
		delete(r.flows, key)
		metrics.ReassemblyActiveFragments.Dec()
	}
}

// cleanup periodically removes expired fragment entries.
func (r *Reassembler) cleanup() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		now := time.Now()
		timeout := time.Duration(r.config.Timeout) * time.Second

		expiredCount := 0
		for key, fl := range r.flows {
			fl.mu.Lock()
			if now.Sub(fl.lastSeen) > timeout {
				delete(r.flows, key)
				expiredCount++
			}
			fl.mu.Unlock()
		}

		if expiredCount > 0 {
			metrics.ReassemblyActiveFragments.Sub(float64(expiredCount))
		}

		r.mu.Unlock()
	}
}
