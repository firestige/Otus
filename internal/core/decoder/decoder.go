// Package decoder implements L2-L4 protocol stack decoding.
package decoder

import (
	"fmt"

	"firestige.xyz/otus/internal/core"
)

// Decoder decodes raw packets into structured format.
type Decoder interface {
	Decode(raw core.RawPacket) (core.DecodedPacket, error)
}

// Config contains decoder configuration.
type Config struct {
	// Tunnels to decapsulate (e.g., "vxlan", "gre", "geneve", "ipip")
	Tunnels []string
	// Enable IP fragment reassembly
	IPReassembly bool
	// Reassembly configuration
	MaxFragments      int // Maximum fragments per flow
	MaxReassembleSize int // Maximum reassembled packet size
	ReassemblyTimeout int // Timeout in seconds
}

// StandardDecoder is the standard implementation of Decoder.
type StandardDecoder struct {
	config      Config
	reassembler *Reassembler // nil if reassembly disabled
	tunnels     map[string]bool
}

// NewStandardDecoder creates a new standard decoder.
func NewStandardDecoder(cfg Config) *StandardDecoder {
	// Default configuration
	if cfg.MaxFragments == 0 {
		cfg.MaxFragments = 100
	}
	if cfg.MaxReassembleSize == 0 {
		cfg.MaxReassembleSize = 65535
	}
	if cfg.ReassemblyTimeout == 0 {
		cfg.ReassemblyTimeout = 60
	}

	sd := &StandardDecoder{
		config:  cfg,
		tunnels: make(map[string]bool),
	}

	// Build tunnel map
	for _, t := range cfg.Tunnels {
		sd.tunnels[t] = true
	}

	// Create reassembler if enabled
	if cfg.IPReassembly {
		sd.reassembler = NewReassembler(ReassemblyConfig{
			MaxFragments:      cfg.MaxFragments,
			MaxReassembleSize: cfg.MaxReassembleSize,
			Timeout:           cfg.ReassemblyTimeout,
		})
	}

	return sd
}

// Decode decodes a raw packet into structured format.
func (sd *StandardDecoder) Decode(raw core.RawPacket) (core.DecodedPacket, error) {
	decoded := core.DecodedPacket{
		Timestamp:  raw.Timestamp,
		CaptureLen: raw.CaptureLen,
		OrigLen:    raw.OrigLen,
	}

	data := raw.Data
	if len(data) == 0 {
		return decoded, fmt.Errorf("empty packet data")
	}

	// L2 Ethernet decoding
	eth, payload, err := decodeEthernet(data)
	if err != nil {
		return decoded, fmt.Errorf("ethernet decode failed: %w", err)
	}
	decoded.Ethernet = eth
	data = payload

	// Check if it's IP packet
	if eth.EtherType != 0x0800 && eth.EtherType != 0x86DD {
		// Non-IP packet, return early
		decoded.Payload = data
		return decoded, nil
	}

	// L3 IP decoding
	ip, payload, err := decodeIP(data)
	if err != nil {
		return decoded, fmt.Errorf("ip decode failed: %w", err)
	}
	decoded.IP = ip
	data = payload

	// Handle tunnels (VXLAN, GRE, etc.)
	if sd.shouldDecapTunnel(ip.Protocol) {
		innerIP, innerPayload, err := decodeTunnel(data, ip.Protocol)
		if err == nil && innerIP.Version != 0 {
			// Successfully decapsulated tunnel
			decoded.IP.InnerSrcIP = innerIP.SrcIP
			decoded.IP.InnerDstIP = innerIP.DstIP
			ip = innerIP
			data = innerPayload
		}
	}

	// Handle IP fragmentation
	if sd.reassembler != nil {
		isFragmented := isIPFragment(raw.Data[:len(raw.Data)-len(data)], ip.Version)
		if isFragmented {
			// Try reassembly
			reassembled, complete, err := sd.reassembler.Process(ip, data, raw.Timestamp)
			if err != nil {
				return decoded, fmt.Errorf("reassembly failed: %w", err)
			}
			if !complete {
				// Fragment not complete yet, return empty
				return decoded, core.ErrFragmentIncomplete
			}
			// Use reassembled data
			data = reassembled
			decoded.Reassembled = true
		}
	}

	// L4 Transport decoding
	if ip.Protocol == 6 || ip.Protocol == 17 { // TCP or UDP
		transport, payload, err := decodeTransport(data, ip.Protocol)
		if err != nil {
			return decoded, fmt.Errorf("transport decode failed: %w", err)
		}
		decoded.Transport = transport
		data = payload
	}

	decoded.Payload = data
	return decoded, nil
}

// shouldDecapTunnel checks if protocol should be decapsulated.
func (sd *StandardDecoder) shouldDecapTunnel(protocol uint8) bool {
	// GRE = 47, UDP (for VXLAN) = 17, IPIP = 4
	if protocol == 47 && sd.tunnels["gre"] {
		return true
	}
	if protocol == 17 && (sd.tunnels["vxlan"] || sd.tunnels["geneve"]) {
		return true
	}
	if protocol == 4 && sd.tunnels["ipip"] {
		return true
	}
	return false
}
