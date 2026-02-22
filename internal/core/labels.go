// Package core defines core types.
package core

// Labels represents key-value metadata attached by parsers and processors.
type Labels map[string]string

// Label naming constants following {protocol}.{field} convention.
const (
	LabelSIPMethod     = "sip.method"
	LabelSIPCallID     = "sip.call_id"
	LabelSIPFromURI    = "sip.from_uri"
	LabelSIPToURI      = "sip.to_uri"
	LabelSIPStatusCode = "sip.status_code"
	LabelSIPVia        = "sip.via" // Comma-separated list of Via headers

	// RTP / RTCP label constants
	LabelRTPVersion     = "rtp.version"
	LabelRTPPayloadType = "rtp.payload_type" // RTP payload type number (0-127)
	LabelRTPSeq         = "rtp.seq"          // Sequence number (decimal)
	LabelRTPTimestamp   = "rtp.timestamp"    // RTP timestamp (decimal)
	LabelRTPSSRC        = "rtp.ssrc"         // Synchronization source (hex, 0xXXXXXXXX)
	LabelRTPCallID      = "rtp.call_id"      // Correlated SIP call-id
	LabelRTPCodec       = "rtp.codec"        // Codec name from SDP (e.g. "PCMU")
	LabelRTPMarker      = "rtp.marker"       // Marker bit ("true"/"false")
	LabelRTPExtension   = "rtp.has_ext"      // Header extension present ("true"/"false")

	// RTCP uses rtcp.* prefix to distinguish from media RTP
	LabelRTCPPayloadType = "rtcp.payload_type" // RTCP packet type (200-209)
	LabelRTCPCallID      = "rtcp.call_id"      // Correlated SIP call-id
	LabelRTCPSSRC        = "rtcp.ssrc"         // Sender/source SSRC (hex)
	LabelRTCPCodec       = "rtcp.codec"        // Codec from SDP for this RTCP flow
	// More labels will be added as protocols are implemented
)
