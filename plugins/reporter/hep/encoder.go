// Package hep implements HEPv3 protocol encoding.
//
// HEP (Homer Encapsulation Protocol) v3 frame layout:
//
//	Offset  Size  Description
//	------  ----  -----------
//	0       4     Magic: "HEP3"
//	4       2     Total frame length (big-endian uint16, includes these 6 bytes)
//	6       …     Chunks (variable count)
//
// Each chunk:
//
//	0  2   Vendor ID  (uint16, 0x0000 = HOMER standard)
//	2  2   Chunk type (uint16)
//	4  2   Total chunk length including this 6-byte header (uint16)
//	6  …   Value (length−6 bytes)
//
// Standard chunk types (vendor 0x0000):
//
//	1   IP family         uint8  (2=IPv4, 10=IPv6)
//	2   IP protocol ID    uint8  (6=TCP, 17=UDP, 132=SCTP)
//	3   Source  IPv4      4 bytes
//	4   Dest    IPv4      4 bytes
//	5   Source  IPv6      16 bytes
//	6   Dest    IPv6      16 bytes
//	7   Source port       uint16
//	8   Dest   port       uint16
//	9   Timestamp sec     uint32
//	10  Timestamp µsec    uint32
//	11  Protocol type     uint8  (1=SIP, 5=RTP, 8=RTCP, 100=JSON)
//	12  Capture agent ID  uint32
//	14  Auth key          string (no NUL terminator)
//	15  Payload           bytes
//	17  Correlation ID    string
//
// Custom (vendor 0x0000, project-specific):
//
//	48  From identity     string  (SIP From-URI or srcIP:port)
//	49  To   identity     string  (SIP To-URI   or dstIP:port)
package hep

import (
	"encoding/binary"
	"fmt"
	"time"

	"icc.tech/capture-agent/internal/core"
)

// ─── HEPv3 constants ───────────────────────────────────────────────────────

const (
	hepMagic = "HEP3"

	// chunkHeaderLen is the fixed overhead of every chunk (vendor + type + length).
	chunkHeaderLen = 6

	// vendorHOMER is the vendor ID used by the HOMER/Sipcapture project.
	vendorHOMER = uint16(0x0000)
)

// Standard chunk type IDs.
const (
	chunkIPFamily  = uint16(1)
	chunkIPProto   = uint16(2)
	chunkSrcIPv4   = uint16(3)
	chunkDstIPv4   = uint16(4)
	chunkSrcIPv6   = uint16(5)
	chunkDstIPv6   = uint16(6)
	chunkSrcPort   = uint16(7)
	chunkDstPort   = uint16(8)
	chunkTimeSec   = uint16(9)
	chunkTimeUsec  = uint16(10)
	chunkProtoType = uint16(11)
	chunkCaptureID = uint16(12)
	chunkAuthKey   = uint16(14)
	chunkPayload   = uint16(15)
	chunkCorrID    = uint16(17)
	chunkNodeName  = uint16(19) // capture node hostname / name

	// Custom chunk IDs (project-specific, per spec).
	chunkFrom = uint16(48) // originating identity (SIP From-URI or srcIP:port)
	chunkTo   = uint16(49) // terminating identity  (SIP To-URI   or dstIP:port)
)

// IP-family values used in chunk 1.
const (
	ipFamilyV4 = uint8(2)
	ipFamilyV6 = uint8(10)
)

// Protocol-type values used in chunk 11.
const (
	protoTypeSIP  = uint8(1)
	protoTypeRTP  = uint8(5)
	protoTypeRTCP = uint8(8)
	protoTypeJSON = uint8(100)
)

// ─── Public encoder ────────────────────────────────────────────────────────

// EncodeOptions carries per-frame knobs that come from reporter config.
type EncodeOptions struct {
	CaptureID uint32 // chunk 12 — agent identifier
	AuthKey   string // chunk 14 — optional authentication key
	NodeName  string // chunk 19 — capture node name / hostname (omitted if empty)
}

// Encode serialises pkt into a HEPv3 byte frame.
// The caller owns the returned slice; it must not be modified after writing to UDP.
func Encode(pkt *core.OutputPacket, opts EncodeOptions) ([]byte, error) {
	if pkt == nil {
		return nil, fmt.Errorf("hep: nil packet")
	}

	// Pre-allocate a generous buffer to avoid re-allocations for typical SIP/RTP frames.
	buf := make([]byte, 0, 512+len(pkt.RawPayload))

	// Frame header — magic + 2-byte length placeholder (filled at end).
	buf = append(buf, hepMagic...)
	buf = append(buf, 0, 0) // bytes [4:6] — total length

	// ── Chunk 1: IP family ──────────────────────────────────────────────────
	ipFamily := ipFamilyV4
	if pkt.SrcIP.Is6() {
		ipFamily = ipFamilyV6
	}
	buf = appendUint8(buf, chunkIPFamily, ipFamily)

	// ── Chunk 2: IP protocol ────────────────────────────────────────────────
	buf = appendUint8(buf, chunkIPProto, pkt.Protocol)

	// ── Chunks 3/4 or 5/6: IP addresses ────────────────────────────────────
	if ipFamily == ipFamilyV4 {
		src4 := pkt.SrcIP.As4()
		dst4 := pkt.DstIP.As4()
		buf = appendBytes(buf, chunkSrcIPv4, src4[:])
		buf = appendBytes(buf, chunkDstIPv4, dst4[:])
	} else {
		src6 := pkt.SrcIP.As16()
		dst6 := pkt.DstIP.As16()
		buf = appendBytes(buf, chunkSrcIPv6, src6[:])
		buf = appendBytes(buf, chunkDstIPv6, dst6[:])
	}

	// ── Chunk 7: source port ────────────────────────────────────────────────
	buf = appendUint16(buf, chunkSrcPort, pkt.SrcPort)

	// ── Chunk 8: destination port ───────────────────────────────────────────
	buf = appendUint16(buf, chunkDstPort, pkt.DstPort)

	// ── Chunks 9/10: timestamp ──────────────────────────────────────────────
	ts := pkt.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	buf = appendUint32(buf, chunkTimeSec, uint32(ts.Unix()))
	buf = appendUint32(buf, chunkTimeUsec, uint32(ts.Nanosecond()/1_000))

	// ── Chunk 11: protocol type ─────────────────────────────────────────────
	buf = appendUint8(buf, chunkProtoType, resolveProtoType(pkt.PayloadType))

	// ── Chunk 12: capture agent ID ──────────────────────────────────────────
	buf = appendUint32(buf, chunkCaptureID, opts.CaptureID)

	// ── Chunk 14: auth key (optional) ───────────────────────────────────────
	if opts.AuthKey != "" {
		buf = appendBytes(buf, chunkAuthKey, []byte(opts.AuthKey))
	}

	// ── Chunk 15: raw payload ────────────────────────────────────────────────
	if len(pkt.RawPayload) > 0 {
		buf = appendBytes(buf, chunkPayload, pkt.RawPayload)
	}

	// ── Chunk 17: correlation ID ─────────────────────────────────────────────
	if cid := resolveCorrelationID(pkt); cid != "" {
		buf = appendBytes(buf, chunkCorrID, []byte(cid))
	}

	// ── Chunk 19: node name ──────────────────────────────────────────────────
	if opts.NodeName != "" {
		buf = appendBytes(buf, chunkNodeName, []byte(opts.NodeName))
	}

	// ── Chunk 48: from identity ──────────────────────────────────────────────
	if from := resolveFrom(pkt); from != "" {
		buf = appendBytes(buf, chunkFrom, []byte(from))
	}

	// ── Chunk 49: to identity ────────────────────────────────────────────────
	if to := resolveTo(pkt); to != "" {
		buf = appendBytes(buf, chunkTo, []byte(to))
	}

	// Back-fill total frame length.
	if len(buf) > 0xFFFF {
		return nil, fmt.Errorf("hep: frame too large (%d bytes, max 65535)", len(buf))
	}
	binary.BigEndian.PutUint16(buf[4:6], uint16(len(buf)))

	return buf, nil
}

// ─── Resolution helpers ────────────────────────────────────────────────────

// resolveProtoType maps a parser PayloadType string to HEP protocol type ID.
func resolveProtoType(payloadType string) uint8 {
	switch payloadType {
	case "sip":
		return protoTypeSIP
	case "rtp":
		return protoTypeRTP
	case "rtcp":
		return protoTypeRTCP
	case "json":
		return protoTypeJSON
	default:
		return 0
	}
}

// resolveFrom extracts the originating identity for chunk 48.
// Priority: SIP From-URI label → srcIP:srcPort.
func resolveFrom(pkt *core.OutputPacket) string {
	if v := pkt.Labels[core.LabelSIPFromURI]; v != "" {
		return v
	}
	return fmt.Sprintf("%s:%d", pkt.SrcIP, pkt.SrcPort)
}

// resolveTo extracts the terminating identity for chunk 49.
// Priority: SIP To-URI label → dstIP:dstPort.
func resolveTo(pkt *core.OutputPacket) string {
	if v := pkt.Labels[core.LabelSIPToURI]; v != "" {
		return v
	}
	return fmt.Sprintf("%s:%d", pkt.DstIP, pkt.DstPort)
}

// resolveCorrelationID returns a call/session correlation string for chunk 17.
// Prefers SIP call-id, then RTP call-id, then TaskID.
func resolveCorrelationID(pkt *core.OutputPacket) string {
	if v := pkt.Labels[core.LabelSIPCallID]; v != "" {
		return v
	}
	if v := pkt.Labels[core.LabelRTPCallID]; v != "" {
		return v
	}
	return pkt.TaskID
}

// ─── Low-level chunk builders ──────────────────────────────────────────────

// appendChunkHeader writes the 6-byte chunk header (vendor, type, totalLen).
func appendChunkHeader(buf []byte, chunkType uint16, valueLen int) []byte {
	var h [chunkHeaderLen]byte
	binary.BigEndian.PutUint16(h[0:2], vendorHOMER)
	binary.BigEndian.PutUint16(h[2:4], chunkType)
	binary.BigEndian.PutUint16(h[4:6], uint16(chunkHeaderLen+valueLen))
	return append(buf, h[:]...)
}

// appendBytes writes a variable-length string/bytes chunk.
func appendBytes(buf []byte, chunkType uint16, value []byte) []byte {
	buf = appendChunkHeader(buf, chunkType, len(value))
	return append(buf, value...)
}

// appendUint8 writes a 1-byte value chunk.
func appendUint8(buf []byte, chunkType uint16, value uint8) []byte {
	buf = appendChunkHeader(buf, chunkType, 1)
	return append(buf, value)
}

// appendUint16 writes a 2-byte big-endian value chunk.
func appendUint16(buf []byte, chunkType uint16, value uint16) []byte {
	buf = appendChunkHeader(buf, chunkType, 2)
	var v [2]byte
	binary.BigEndian.PutUint16(v[:], value)
	return append(buf, v[:]...)
}

// appendUint32 writes a 4-byte big-endian value chunk.
func appendUint32(buf []byte, chunkType uint16, value uint32) []byte {
	buf = appendChunkHeader(buf, chunkType, 4)
	var v [4]byte
	binary.BigEndian.PutUint32(v[:], value)
	return append(buf, v[:]...)
}
