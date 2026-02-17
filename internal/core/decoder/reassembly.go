// Package decoder implements protocol decoding.
package decoder

import (
	"fmt"
	"sync"
	"time"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/internal/metrics"
)

// ReassemblyConfig contains configuration for IP reassembly.
type ReassemblyConfig struct {
	MaxFragments      int // Maximum fragments per flow
	MaxReassembleSize int // Maximum reassembled packet size
	Timeout           int // Timeout in seconds
}

// fragmentKey uniquely identifies a fragmented packet.
type fragmentKey struct {
	srcIP    string
	dstIP    string
	protocol uint8
	id       uint16 // Fragment ID
}

// fragmentEntry represents a collection of fragments for reassembly.
type fragmentEntry struct {
	fragments   map[uint16][]byte // offset -> data
	totalSize   int               // Expected total size (-1 if unknown)
	lastSeen    time.Time
	hasLastFrag bool // Whether we've seen the last fragment
}

// Reassembler handles IP fragment reassembly.
type Reassembler struct {
	config ReassemblyConfig
	mu     sync.Mutex
	flows  map[fragmentKey]*fragmentEntry
}

// NewReassembler creates a new IP fragment reassembler.
func NewReassembler(cfg ReassemblyConfig) *Reassembler {
	r := &Reassembler{
		config: cfg,
		flows:  make(map[fragmentKey]*fragmentEntry),
	}

	// Start cleanup goroutine
	go r.cleanup()

	return r
}

// Process processes a potentially fragmented IP packet.
// Returns reassembled data, whether reassembly is complete, and error.
func (r *Reassembler) Process(ip core.IPHeader, data []byte, timestamp time.Time) ([]byte, bool, error) {
	// For now, only support IPv4 fragmentation
	if ip.Version != 4 {
		return data, true, nil
	}

	// Create fragment key
	key := fragmentKey{
		srcIP:    ip.SrcIP.String(),
		dstIP:    ip.DstIP.String(),
		protocol: ip.Protocol,
		// TODO: Extract fragment ID from IP header
		// For now, use a placeholder
		id: 0,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Get or create fragment entry
	entry, exists := r.flows[key]
	if !exists {
		entry = &fragmentEntry{
			fragments: make(map[uint16][]byte),
			totalSize: -1,
			lastSeen:  timestamp,
		}
		r.flows[key] = entry

		// Update Prometheus metric
		metrics.ReassemblyActiveFragments.Inc()
	}

	// Check fragment limit
	if len(entry.fragments) >= r.config.MaxFragments {
		delete(r.flows, key)
		metrics.ReassemblyActiveFragments.Dec()
		return nil, false, core.ErrReassemblyLimit
	}

	// TODO: Parse fragment offset and more fragments flag from IP header
	// For now, simplified implementation
	offset := uint16(0)
	moreFragments := false

	// Store fragment
	entry.fragments[offset] = data
	entry.lastSeen = timestamp

	if !moreFragments {
		entry.hasLastFrag = true
		// Calculate total size
		entry.totalSize = int(offset) + len(data)
	}

	// Check if reassembly is complete
	if entry.hasLastFrag && r.isComplete(entry) {
		// Reassemble
		reassembled, err := r.reassemble(entry)
		delete(r.flows, key)
		metrics.ReassemblyActiveFragments.Dec()
		if err != nil {
			return nil, false, err
		}
		return reassembled, true, nil
	}

	// Not complete yet
	return nil, false, nil
}

// isComplete checks if all fragments have been received.
func (r *Reassembler) isComplete(entry *fragmentEntry) bool {
	if !entry.hasLastFrag || entry.totalSize < 0 {
		return false
	}

	// Calculate received bytes
	receivedBytes := 0
	for _, frag := range entry.fragments {
		receivedBytes += len(frag)
	}

	return receivedBytes >= entry.totalSize
}

// reassemble combines fragments into a single packet.
func (r *Reassembler) reassemble(entry *fragmentEntry) ([]byte, error) {
	if entry.totalSize > r.config.MaxReassembleSize {
		return nil, fmt.Errorf("reassembled size %d exceeds limit %d", entry.totalSize, r.config.MaxReassembleSize)
	}

	// Allocate buffer for reassembled packet
	result := make([]byte, entry.totalSize)

	// Copy fragments into result buffer
	for offset, frag := range entry.fragments {
		copy(result[offset:], frag)
	}

	return result, nil
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
		for key, entry := range r.flows {
			if now.Sub(entry.lastSeen) > timeout {
				delete(r.flows, key)
				expiredCount++
			}
		}

		// Update Prometheus metric for expired fragments
		if expiredCount > 0 {
			metrics.ReassemblyActiveFragments.Sub(float64(expiredCount))
		}

		r.mu.Unlock()
	}
}
