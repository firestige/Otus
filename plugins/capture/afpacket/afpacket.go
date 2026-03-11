//go:build linux

// Package afpacket implements AF_PACKET_V3 capture plugin.
package afpacket

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"golang.org/x/net/bpf"

	"icc.tech/capture-agent/internal/core"
	"icc.tech/capture-agent/pkg/plugin"
)

const (
	pluginName = "afpacket"

	// Default configuration values.
	// snap_len is 65536 (= 2^16 = 16 × 4096) rather than the common-but-problematic
	// 65535: gopacket requires block_size % frame_size == 0 AND block_size % pageSize == 0.
	// 65535 is odd so gcd(65535, 4096)=1 → lcm ≈ 256 MB, making any practical block_size
	// impossible to align.  65536 is page-aligned so the two constraints collapse to one.
	defaultSnapLen    = 65536
	defaultBlockSize  = 4 * 1024 * 1024 // 4MB = 64 × 65536, satisfies default alignment
	defaultNumBlocks  = 128
	defaultFanoutID   = 42
	defaultFanoutType = "hash"
)

// alignSnapLen rounds n up to the nearest multiple of pageSize.
// The result becomes TPacket's frame_size.  Because gopacket enforces both
//
//	block_size % frame_size == 0
//	block_size % pageSize  == 0
//
// making frame_size a multiple of pageSize collapses both constraints into a
// single one (block_size % frame_size == 0), which is easy to satisfy.
func alignSnapLen(n, pageSize int) int {
	if n <= 0 {
		n = pageSize
	}
	// Ceiling division: ((n - 1) / pageSize + 1) * pageSize
	return ((n + pageSize - 1) / pageSize) * pageSize
}

// alignBlockSize rounds requested up to the nearest multiple of frameSize.
// frameSize must already be page-aligned (call alignSnapLen first); that
// guarantees the result also satisfies the pageSize divisibility requirement.
func alignBlockSize(requested, frameSize int) int {
	if frameSize <= 0 {
		frameSize = os.Getpagesize()
	}
	if requested <= 0 {
		return frameSize
	}
	return ((requested + frameSize - 1) / frameSize) * frameSize
}

// Config represents afpacket-specific configuration.
type Config struct {
	Interface   string `json:"interface"`   // required
	BPFFilter   string `json:"bpf_filter"`  // optional
	SnapLen     int    `json:"snap_len"`    // optional, default 65535
	BlockSize   int    `json:"block_size"`  // optional, default 4MB
	NumBlocks   int    `json:"num_blocks"`  // optional, default 128
	FanoutID    int    `json:"fanout_id"`   // optional, default 42
	FanoutType  string `json:"fanout_type"` // optional: hash|cpu|lb, default hash
	Promiscuous bool   `json:"promiscuous"` // optional, default true
}

// AFPacketCapturer implements the Capturer interface using AF_PACKET_V3.
type AFPacketCapturer struct {
	name   string
	config Config

	// Runtime state
	handle *afpacket.TPacket
	ctx    context.Context
	cancel context.CancelFunc

	// Statistics (atomic counters)
	packetsReceived  atomic.Uint64
	packetsDropped   atomic.Uint64
	packetsIfDropped atomic.Uint64
}

// NewAFPacketCapturer creates a new AF_PACKET capturer instance.
func NewAFPacketCapturer() plugin.Capturer {
	return &AFPacketCapturer{
		name: pluginName,
	}
}

// Name returns the plugin name.
func (c *AFPacketCapturer) Name() string {
	return c.name
}

// Init initializes the capturer with configuration.
func (c *AFPacketCapturer) Init(cfg map[string]any) error {
	// Parse configuration
	c.config = Config{
		SnapLen:     defaultSnapLen,
		BlockSize:   defaultBlockSize,
		NumBlocks:   defaultNumBlocks,
		FanoutID:    defaultFanoutID,
		FanoutType:  defaultFanoutType,
		Promiscuous: true,
	}

	if iface, ok := cfg["interface"].(string); ok {
		c.config.Interface = iface
	} else {
		return fmt.Errorf("afpacket: interface is required")
	}

	if filter, ok := cfg["bpf_filter"].(string); ok {
		c.config.BPFFilter = filter
	}

	// ── snap_len (= TPacket frame_size) must be resolved and page-aligned BEFORE
	//    block_size, because block_size alignment depends on the final snap_len value.
	if snapLen, ok := cfg["snap_len"].(float64); ok {
		c.config.SnapLen = int(snapLen)
	}
	page := os.Getpagesize()
	alignedSnap := alignSnapLen(c.config.SnapLen, page)
	if alignedSnap != c.config.SnapLen {
		slog.Warn("afpacket: snap_len rounded up to page boundary to satisfy TPacket alignment",
			"requested", c.config.SnapLen, "aligned", alignedSnap, "page_size", page)
		c.config.SnapLen = alignedSnap
	}

	// ── block_size must be divisible by both snap_len and pageSize.
	//    Since snap_len is now page-aligned, rounding block_size up to the nearest
	//    multiple of snap_len satisfies both constraints simultaneously.
	if blockSize, ok := cfg["block_size"].(float64); ok {
		requested := int(blockSize)
		aligned := alignBlockSize(requested, c.config.SnapLen)
		if aligned != requested {
			slog.Warn("afpacket: block_size rounded up to nearest multiple of snap_len",
				"requested", requested, "aligned", aligned, "snap_len", c.config.SnapLen)
		}
		c.config.BlockSize = aligned
	} else {
		// Re-align the default block_size for the (possibly user-adjusted) snap_len.
		c.config.BlockSize = alignBlockSize(c.config.BlockSize, c.config.SnapLen)
	}

	if numBlocks, ok := cfg["num_blocks"].(float64); ok {
		c.config.NumBlocks = int(numBlocks)
	}

	if fanoutID, ok := cfg["fanout_id"].(float64); ok {
		c.config.FanoutID = int(fanoutID)
	}

	if fanoutType, ok := cfg["fanout_type"].(string); ok {
		c.config.FanoutType = fanoutType
	}

	if promisc, ok := cfg["promiscuous"].(bool); ok {
		c.config.Promiscuous = promisc
	}

	slog.Debug("afpacket initialized",
		"interface", c.config.Interface,
		"bpf_filter", c.config.BPFFilter,
		"snap_len", c.config.SnapLen,
		"fanout_id", c.config.FanoutID,
		"fanout_type", c.config.FanoutType)

	return nil
}

// Start starts the capturer (no-op for afpacket, actual work in Capture).
func (c *AFPacketCapturer) Start(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	return nil
}

// Stop stops the capturer by cancelling the context.
//
// NOTE: handle.Close() is intentionally NOT called here.
// The TPacket handle is owned exclusively by Capture(), which closes it
// via defer once the read loop detects context cancellation and returns.
// Calling Close() here would race with ZeroCopyReadPacketData() inside
// the Capture loop, causing a Use-After-Free SIGSEGV against the
// TPACKET_V3 mmap ring buffer.
func (c *AFPacketCapturer) Stop(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

// Capture captures packets from the network interface.
// This is a blocking call that runs until ctx is cancelled or an error occurs.
func (c *AFPacketCapturer) Capture(ctx context.Context, output chan<- core.RawPacket) error {
	// Create TPacket handle
	opts := []interface{}{
		afpacket.OptInterface(c.config.Interface),
		afpacket.OptFrameSize(c.config.SnapLen),
		afpacket.OptBlockSize(c.config.BlockSize),
		afpacket.OptNumBlocks(c.config.NumBlocks),
		afpacket.OptPollTimeout(100 * time.Millisecond),
		afpacket.OptTPacketVersion(afpacket.TPacketVersion3),
	}

	handle, err := afpacket.NewTPacket(opts...)
	if err != nil {
		return fmt.Errorf("failed to create TPacket handle: %w", err)
	}
	c.handle = handle
	defer func() {
		c.handle.Close()
		c.handle = nil
	}()

	// Set fanout mode if specified
	if c.config.FanoutType != "" {
		fanoutType, err := parseFanoutType(c.config.FanoutType)
		if err != nil {
			return fmt.Errorf("invalid fanout_type: %w", err)
		}
		if err := c.handle.SetFanout(fanoutType, uint16(c.config.FanoutID)); err != nil {
			return fmt.Errorf("failed to set fanout: %w", err)
		}
		slog.Info("afpacket fanout configured",
			"interface", c.config.Interface,
			"fanout_id", c.config.FanoutID,
			"fanout_type", c.config.FanoutType)
	}

	slog.Info("afpacket capture started", "interface", c.config.Interface)

	// Apply BPF filter if specified
	if c.config.BPFFilter != "" {
		if err := c.applyBPFFilter(); err != nil {
			return fmt.Errorf("failed to apply BPF filter: %w", err)
		}
		slog.Debug("BPF filter applied", "filter", c.config.BPFFilter)
	}

	// Initialize socket stats
	if err := c.handle.InitSocketStats(); err != nil {
		slog.Warn("failed to init socket stats", "error", err)
	}

	// Direct read loop — bypasses gopacket.PacketSource.Packets() which spawns a
	// hidden goroutine that continues accessing the TPACKET_V3 mmap ring buffer
	// after handle.Close() unmaps it, causing a Use-After-Free SIGSEGV.
	//
	// By calling ZeroCopyReadPacketData() directly, there are no hidden goroutines
	// and the handle lifetime is fully controlled by this function's defer above.
	for {
		// Check for shutdown before each blocking read so we react promptly
		// when context is cancelled between poll timeouts.
		// Use c.ctx (capturer-own context) so Stop() → c.cancel() exits this loop
		// without waiting for the parent task context to be cancelled.
		select {
		case <-c.ctx.Done():
			slog.Info("afpacket capture stopped", "interface", c.config.Interface)
			return nil
		default:
		}

		data, ci, err := c.handle.ZeroCopyReadPacketData()
		if err != nil {
			// On any read error, check context first (covers poll timeout, EAGAIN, etc.).
			if c.ctx.Err() != nil {
				slog.Info("afpacket capture stopped", "interface", c.config.Interface)
				return nil
			}
			// Transient errors (poll timeout OptPollTimeout=100ms, EINTR, etc.) — retry.
			continue
		}

		// Update statistics
		c.packetsReceived.Add(1)

		// Update drop counters from socket stats
		if socketStats, _, statsErr := c.handle.SocketStats(); statsErr == nil {
			c.packetsDropped.Store(uint64(socketStats.Drops()))
		}

		// Build RawPacket from zero-copy ring-buffer data.
		// NOTE: data is only valid until the next ZeroCopyReadPacketData call;
		// the pipeline must consume or copy it before we loop (same contract as
		// the previous PacketSource NoCopy=true approach).
		raw := core.RawPacket{
			Data:           data,
			Timestamp:      ci.Timestamp,
			CaptureLen:     uint32(ci.CaptureLength),
			OrigLen:        uint32(ci.Length),
			InterfaceIndex: ci.InterfaceIndex,
		}

		// Non-blocking send: prefer drop over blocking the read loop.
		// c.ctx.Done() guards against the channel being closed before we exit.
		select {
		case output <- raw:
			// Packet sent successfully
		case <-c.ctx.Done():
			slog.Info("afpacket capture stopped", "interface", c.config.Interface)
			return nil
		default:
			// Output channel full, drop packet
			c.packetsDropped.Add(1)
			slog.Debug("output channel full, dropping packet",
				"interface", c.config.Interface)
		}
	}
}

// applyBPFFilter compiles and applies a BPF filter to the capture handle.
func (c *AFPacketCapturer) applyBPFFilter() error {
	// Compile BPF filter using pcap (returns pcap.BPFInstruction slice)
	pcapInsns, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, c.config.SnapLen, c.config.BPFFilter)
	if err != nil {
		return fmt.Errorf("failed to compile BPF filter %q: %w", c.config.BPFFilter, err)
	}

	// Convert pcap.BPFInstruction to bpf.RawInstruction
	// The structures are identical: Code->Op, Jt, Jf, K
	rawInsns := make([]bpf.RawInstruction, len(pcapInsns))
	for i, insn := range pcapInsns {
		rawInsns[i] = bpf.RawInstruction{
			Op: insn.Code,
			Jt: insn.Jt,
			Jf: insn.Jf,
			K:  insn.K,
		}
	}

	// Apply to TPacket handle
	if err := c.handle.SetBPF(rawInsns); err != nil {
		return fmt.Errorf("failed to set BPF: %w", err)
	}

	return nil
}

// Stats returns capture statistics.
func (c *AFPacketCapturer) Stats() plugin.CaptureStats {
	return plugin.CaptureStats{
		PacketsReceived:  c.packetsReceived.Load(),
		PacketsDropped:   c.packetsDropped.Load(),
		PacketsIfDropped: c.packetsIfDropped.Load(),
	}
}

// parseFanoutType converts fanout type string to afpacket constant.
//
// Limitation: gopacket/afpacket v1.1.19 only exports FanoutHash.
// The Linux kernel supports additional modes (PACKET_FANOUT_CPU,
// PACKET_FANOUT_LB, PACKET_FANOUT_ROLLOVER, etc.), but gopacket's
// afpacket package does not expose them as typed constants.
// To support "cpu" or "lb" modes, either:
//   - Upgrade to a gopacket version that exports them, or
//   - Use golang.org/x/sys/unix.PACKET_FANOUT_* with raw socket syscalls
//     (bypassing gopacket's TPacket abstraction).
//
// Architecture doc lists "hash | cpu | lb" — currently only "hash" is implemented.
func parseFanoutType(ft string) (afpacket.FanoutType, error) {
	switch ft {
	case "hash":
		return afpacket.FanoutHash, nil
	case "":
		// Empty string means no fanout
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown fanout type: %q (only 'hash' is supported; see code comments for limitation details)", ft)
	}
}
