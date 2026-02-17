// Package afpacket implements AF_PACKET_V3 capture plugin.
package afpacket

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"golang.org/x/net/bpf"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/pkg/plugin"
)

const (
	pluginName = "afpacket"

	// Default configuration values
	defaultSnapLen    = 65535
	defaultBlockSize  = 4 * 1024 * 1024 // 4MB
	defaultNumBlocks  = 128
	defaultFanoutID   = 42
	defaultFanoutType = "hash"
)

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

	if snapLen, ok := cfg["snap_len"].(float64); ok {
		c.config.SnapLen = int(snapLen)
	}

	if blockSize, ok := cfg["block_size"].(float64); ok {
		c.config.BlockSize = int(blockSize)
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

// Stop stops the capturer.
func (c *AFPacketCapturer) Stop(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}

	if c.handle != nil {
		c.handle.Close()
		c.handle = nil
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
	defer c.handle.Close()

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

	// Packet source
	packetSource := gopacket.NewPacketSource(c.handle, layers.LinkTypeEthernet)
	packetSource.NoCopy = true // Zero-copy mode

	// Initialize socket stats
	if err := c.handle.InitSocketStats(); err != nil {
		slog.Warn("failed to init socket stats", "error", err)
	}

	// Capture loop
	for {
		select {
		case <-ctx.Done():
			slog.Info("afpacket capture stopped", "interface", c.config.Interface)
			return nil

		case packet, ok := <-packetSource.Packets():
			if !ok {
				// Channel closed
				return fmt.Errorf("packet source channel closed")
			}

			// Update statistics
			c.packetsReceived.Add(1)

			// Update drop counters from socket stats
			if socketStats, _, statsErr := c.handle.SocketStats(); statsErr == nil {
				c.packetsDropped.Store(uint64(socketStats.Drops()))
			}

			// Build RawPacket
			raw := core.RawPacket{
				Data:           packet.Data(),
				Timestamp:      packet.Metadata().Timestamp,
				CaptureLen:     uint32(packet.Metadata().CaptureLength),
				OrigLen:        uint32(packet.Metadata().Length),
				InterfaceIndex: packet.Metadata().InterfaceIndex,
			}

			// Non-blocking send to output channel
			select {
			case output <- raw:
				// Packet sent successfully
			case <-ctx.Done():
				return nil
			default:
				// Output channel full, drop packet
				c.packetsDropped.Add(1)
				slog.Debug("output channel full, dropping packet",
					"interface", c.config.Interface)
			}
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
func parseFanoutType(ft string) (afpacket.FanoutType, error) {
	switch ft {
	case "hash":
		return afpacket.FanoutHash, nil
	case "":
		// Empty string means no fanout
		return 0, nil
	default:
		// Note: gopacket v1.1.19 only exposes FanoutHash in the public API.
		// Other fanout types (cpu, lb, rollover, etc.) are not available
		// as named constants. For production use, consider using a newer
		// version or accessing unix.PACKET_FANOUT_* constants directly.
		return 0, fmt.Errorf("unknown fanout type: %q (only 'hash' is supported)", ft)
	}
}
