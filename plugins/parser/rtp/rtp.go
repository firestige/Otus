// Package rtp implements an RTP/RTCP protocol parser.
//
// The parser correlates media flows with SIP sessions via the shared FlowRegistry
// that is populated by the SIP parser when it processes INVITE / 200 OK SDP exchange.
// Two operating modes:
//
//  1. FlowRegistry hit (fast path): The 5-tuple was pre-registered by the SIP parser,
//     so the parser knows immediately that the packet is RTP/RTCP and has call context.
//
//  2. Heuristic fallback: No FlowRegistry entry.  The parser applies lightweight header
//     checks (V=2, payload-type range, minimum length) to decide whether the datagram
//     looks like RTP or RTCP.
//
// RTCP is distinguished from RTP by payload-type values 200–209 (SR, RR, SDES, BYE…).
package rtp

import (
	"context"
	"encoding/binary"
	"fmt"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/pkg/plugin"
)

// rtcpPayloadTypeMin / Max define the RTCP PT range per RFC 5761 / RFC 3550.
const (
	rtcpPayloadTypeMin = 200
	rtcpPayloadTypeMax = 209

	rtpMinLength  = 12 // Fixed RTP header size (RFC 3550 §5.1)
	rtcpMinLength = 8  // Fixed RTCP common header + sender SSRC
)

// RTPParser parses RTP and RTCP datagrams.
//
// It implements plugin.Parser and plugin.FlowRegistryAware.
type RTPParser struct {
	name         string
	flowRegistry plugin.FlowRegistry
}

// NewRTPParser creates a new RTPParser instance.
func NewRTPParser() plugin.Parser {
	return &RTPParser{name: "rtp"}
}

// Name returns the plugin identifier used in task configuration.
func (p *RTPParser) Name() string { return p.name }

// Init initialises the parser; no configuration is required for Phase 2.
func (p *RTPParser) Init(_ map[string]any) error { return nil }

// Start is a no-op — RTPParser has no goroutines or background resources.
func (p *RTPParser) Start(_ context.Context) error { return nil }

// Stop is a no-op for the same reason.
func (p *RTPParser) Stop(_ context.Context) error { return nil }

// SetFlowRegistry satisfies plugin.FlowRegistryAware.
// The task manager calls this during wire-up so that RTPParser shares the
// same FlowRegistry instance as the SIP parser in the same Task.
func (p *RTPParser) SetFlowRegistry(registry plugin.FlowRegistry) {
	p.flowRegistry = registry
}

// CanHandle decides whether the packet should be processed by this parser.
//
// Decision order (cheapest first):
//  1. If the FlowRegistry has an entry for this 5-tuple the answer is definitely yes.
//  2. Otherwise apply a lightweight header heuristic: V=2, appropriate PT range, min len.
func (p *RTPParser) CanHandle(pkt *core.DecodedPacket) bool {
	// Only UDP is relevant for RTP/RTCP.
	if pkt.Transport.Protocol != 17 {
		return false
	}

	// Fast path: FlowRegistry lookup — O(1), zero allocation.
	if p.flowRegistry != nil {
		key := plugin.FlowKey{
			SrcIP:   pkt.IP.SrcIP,
			DstIP:   pkt.IP.DstIP,
			SrcPort: pkt.Transport.SrcPort,
			DstPort: pkt.Transport.DstPort,
			Proto:   17,
		}
		if _, ok := p.flowRegistry.Get(key); ok {
			return true
		}
	}

	// Heuristic fallback — check shared RTP/RTCP fixed header fields.
	return looksLikeRTPorRTCP(pkt.Payload)
}

// Handle parses the RTP or RTCP header and returns annotated labels.
//
// The payload (first return value) is nil — all metadata is surfaced as labels,
// consistent with the SIP parser's convention.
func (p *RTPParser) Handle(pkt *core.DecodedPacket) (any, core.Labels, error) {
	if len(pkt.Payload) < 2 {
		return nil, nil, fmt.Errorf("rtp: payload too short (%d bytes)", len(pkt.Payload))
	}

	// Byte 0: V(2) P(1) X(1) CC(4)
	// Byte 1: M(1) PT(7)  — for RTP;  RC(5) PT(8) — for RTCP
	pt := pkt.Payload[1] & 0x7F // mask out Marker bit for RTP; mask is same position for RTCP

	// RTCP packets have PT in the range 200–209 and the marker-bit position is
	// used for the RC/SC count field, but the PT is in byte 1 without masking
	// the high bit for RTCP (RFC 3550 §6.4 uses full byte 1).
	rtcpPT := pkt.Payload[1] // unmasked for RTCP detection
	if rtcpPT >= rtcpPayloadTypeMin && rtcpPT <= rtcpPayloadTypeMax {
		return p.handleRTCP(pkt, rtcpPT)
	}

	return p.handleRTP(pkt, pt)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// handleRTP parses the 12-byte fixed RTP header and populates labels.
func (p *RTPParser) handleRTP(pkt *core.DecodedPacket, pt uint8) (any, core.Labels, error) {
	if len(pkt.Payload) < rtpMinLength {
		return nil, nil, fmt.Errorf("rtp: payload too short for RTP header (%d bytes)", len(pkt.Payload))
	}

	b := pkt.Payload

	// Byte 0: V(7:6) P(5) X(4) CC(3:0)
	version := (b[0] >> 6) & 0x3
	if version != 2 {
		return nil, nil, fmt.Errorf("rtp: unexpected RTP version %d", version)
	}
	hasExtension := (b[0]>>4)&0x1 == 1
	marker := (b[1]>>7)&0x1 == 1

	// Bytes 2–3: sequence number
	seq := binary.BigEndian.Uint16(b[2:4])

	// Bytes 4–7: timestamp
	ts := binary.BigEndian.Uint32(b[4:8])

	// Bytes 8–11: SSRC
	ssrc := binary.BigEndian.Uint32(b[8:12])

	labels := core.Labels{
		core.LabelRTPVersion:     fmt.Sprintf("%d", version),
		core.LabelRTPPayloadType: fmt.Sprintf("%d", pt),
		core.LabelRTPSeq:         fmt.Sprintf("%d", seq),
		core.LabelRTPTimestamp:   fmt.Sprintf("%d", ts),
		core.LabelRTPSSRC:        fmt.Sprintf("0x%08X", ssrc),
		core.LabelRTPMarker:      boolStr(marker),
		core.LabelRTPExtension:   boolStr(hasExtension),
	}

	// Enrich with SIP call context from FlowRegistry.
	p.enrichFromRegistry(pkt, labels, false)

	return nil, labels, nil
}

// handleRTCP parses the 8-byte RTCP common header and populates labels.
func (p *RTPParser) handleRTCP(pkt *core.DecodedPacket, pt uint8) (any, core.Labels, error) {
	if len(pkt.Payload) < rtcpMinLength {
		return nil, nil, fmt.Errorf("rtp: payload too short for RTCP header (%d bytes)", len(pkt.Payload))
	}

	b := pkt.Payload

	version := (b[0] >> 6) & 0x3
	if version != 2 {
		return nil, nil, fmt.Errorf("rtp: unexpected RTCP version %d", version)
	}

	// Bytes 4–7: SSRC of sender (SR/RR) or first SSRC (SDES/BYE/APP)
	ssrc := binary.BigEndian.Uint32(b[4:8])

	labels := core.Labels{
		core.LabelRTCPPayloadType: fmt.Sprintf("%d", pt),
		core.LabelRTCPSSRC:       fmt.Sprintf("0x%08X", ssrc),
	}

	// Enrich with SIP call context from FlowRegistry.
	p.enrichFromRegistry(pkt, labels, true)

	return nil, labels, nil
}

// enrichFromRegistry looks up the FlowRegistry and adds call_id / codec labels.
// isRTCP controls which label keys to use (rtcp.* vs rtp.*).
func (p *RTPParser) enrichFromRegistry(pkt *core.DecodedPacket, labels core.Labels, isRTCP bool) {
	if p.flowRegistry == nil {
		return
	}

	key := plugin.FlowKey{
		SrcIP:   pkt.IP.SrcIP,
		DstIP:   pkt.IP.DstIP,
		SrcPort: pkt.Transport.SrcPort,
		DstPort: pkt.Transport.DstPort,
		Proto:   17,
	}

	val, ok := p.flowRegistry.Get(key)
	if !ok {
		return
	}

	ctx, ok := val.(map[string]string)
	if !ok {
		return
	}

	if isRTCP {
		if callID, ok := ctx["call_id"]; ok && callID != "" {
			labels[core.LabelRTCPCallID] = callID
		}
		if codec, ok := ctx["codec"]; ok && codec != "" {
			labels[core.LabelRTCPCodec] = codec
		}
	} else {
		if callID, ok := ctx["call_id"]; ok && callID != "" {
			labels[core.LabelRTPCallID] = callID
		}
		if codec, ok := ctx["codec"]; ok && codec != "" {
			labels[core.LabelRTPCodec] = codec
		}
	}
}

// looksLikeRTPorRTCP returns true when the payload passes lightweight header checks.
//
// Rules (applies to both RTP and RTCP — the V=2 check is shared):
//   - At least 8 bytes present (shorter RTCP min-size).
//   - First 2 bits (V field) == 0b10 (version 2).
//   - Byte 1 (PT, with or without M bit) must be < 128 for RTP, or 200–209 for RTCP.
func looksLikeRTPorRTCP(payload []byte) bool {
	if len(payload) < rtcpMinLength {
		return false
	}

	v := (payload[0] >> 6) & 0x3
	if v != 2 {
		return false
	}

	rtcpPT := payload[1]
	if rtcpPT >= rtcpPayloadTypeMin && rtcpPT <= rtcpPayloadTypeMax {
		return len(payload) >= rtcpMinLength
	}

	// RTP: marker + PT in byte 1; PT (7 bits) must be < 128 and packet long enough.
	rtpPT := payload[1] & 0x7F
	return rtpPT < 128 && len(payload) >= rtpMinLength
}

// boolStr converts a bool to "true"/"false" string for label values.
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
