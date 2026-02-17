// Package sip implements SIP protocol parser.
// Parses SIP signaling messages and extracts key headers.
// Maintains session state to correlate SDP offers/answers and register media flows.
package sip

import (
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/pkg/plugin"
)

const (
	defaultSessionTTL = 24 * time.Hour
	defaultCleanup    = 1 * time.Hour
)

// SIPParser parses SIP signaling messages.
type SIPParser struct {
	name         string
	sessionCache *cache.Cache        // Call-ID → *sipSession
	flowRegistry plugin.FlowRegistry // Injected via SetFlowRegistry
}

// sipSession tracks SIP call state for correlating INVITE/200 OK.
type sipSession struct {
	callID    string
	offerSDP  *sdpInfo // SDP from INVITE
	answerSDP *sdpInfo // SDP from 200 OK
	createdAt time.Time
}

// sdpInfo contains parsed SDP information.
type sdpInfo struct {
	connectionIP netip.Addr    // c= line IP
	mediaStreams []mediaStream // m= lines
}

// mediaStream represents one m= line with associated a= attributes.
type mediaStream struct {
	mediaType string // "audio" or "video"
	rtpPort   uint16 // RTP port from m= line
	rtcpPort  uint16 // RTCP port (rtpPort+1 or from a=rtcp:)
	rtcpMux   bool   // Whether RTCP is multiplexed on RTP port
	codec     string // From a=rtpmap: (optional, for labels)
	direction string // sendrecv/sendonly/recvonly/inactive
}

// NewSIPParser creates a new SIP parser.
func NewSIPParser() plugin.Parser {
	return &SIPParser{
		name:         "sip",
		sessionCache: cache.New(defaultSessionTTL, defaultCleanup),
	}
}

// Name returns the plugin name.
func (p *SIPParser) Name() string {
	return p.name
}

// Init initializes the parser with configuration.
func (p *SIPParser) Init(config map[string]any) error {
	// Future: configurable TTL, cleanup interval
	return nil
}

// Start starts the parser.
func (p *SIPParser) Start(ctx context.Context) error {
	return nil
}

// Stop stops the parser.
func (p *SIPParser) Stop(ctx context.Context) error {
	p.sessionCache.Flush()
	return nil
}

// SetFlowRegistry sets the flow registry (FlowRegistryAware interface).
func (p *SIPParser) SetFlowRegistry(registry plugin.FlowRegistry) {
	p.flowRegistry = registry
}

// CanHandle checks if this packet is likely SIP.
// Fast check: port 5060/5061 or SIP magic bytes.
func (p *SIPParser) CanHandle(pkt *core.DecodedPacket) bool {
	// Check standard SIP ports
	if pkt.Transport.SrcPort == 5060 || pkt.Transport.DstPort == 5060 ||
		pkt.Transport.SrcPort == 5061 || pkt.Transport.DstPort == 5061 {
		return true
	}

	// Check SIP magic in payload (fast prefix check, no regex)
	if len(pkt.Payload) < 8 {
		return false
	}

	// Check for common SIP method/response prefixes
	prefix := string(pkt.Payload[:8])
	return strings.HasPrefix(prefix, "SIP/2.0 ") ||
		strings.HasPrefix(prefix, "INVITE ") ||
		strings.HasPrefix(prefix, "REGISTER") ||
		strings.HasPrefix(prefix, "BYE ") ||
		strings.HasPrefix(prefix, "CANCEL ") ||
		strings.HasPrefix(prefix, "ACK ") ||
		strings.HasPrefix(prefix, "OPTIONS ") ||
		strings.HasPrefix(prefix, "SUBSCRI") || // SUBSCRIBE
		strings.HasPrefix(prefix, "NOTIFY ")
}

// Handle parses SIP message and extracts labels.
// Manages session state for SDP offer/answer correlation.
func (p *SIPParser) Handle(pkt *core.DecodedPacket) (any, core.Labels, error) {
	labels := make(core.Labels)

	// Parse SIP headers
	sipMsg, err := p.parseSIPMessage(pkt.Payload)
	if err != nil {
		return nil, nil, fmt.Errorf("sip parse failed: %w", err)
	}

	// Populate labels with key headers
	if sipMsg.method != "" {
		labels[core.LabelSIPMethod] = sipMsg.method
	}
	if sipMsg.statusCode != 0 {
		labels[core.LabelSIPStatusCode] = strconv.Itoa(sipMsg.statusCode)
	}
	if sipMsg.callID != "" {
		labels[core.LabelSIPCallID] = sipMsg.callID
	}
	if sipMsg.fromURI != "" {
		labels[core.LabelSIPFromURI] = sipMsg.fromURI
	}
	if sipMsg.toURI != "" {
		labels[core.LabelSIPToURI] = sipMsg.toURI
	}
	if len(sipMsg.viaList) > 0 {
		labels[core.LabelSIPVia] = strings.Join(sipMsg.viaList, ",")
	}

	// Handle session state and flow registration
	// BYE/CANCEL don't require SDP, but INVITE/200 OK do
	if p.flowRegistry != nil {
		p.handleSDP(sipMsg, pkt)
	}

	// No structured payload, only labels (raw payload in OutputPacket.RawPayload)
	return nil, labels, nil
}

// sipMessage represents parsed SIP message.
type sipMessage struct {
	method     string   // Request method (INVITE, BYE, etc.) or empty for response
	statusCode int      // Response status code or 0 for request
	callID     string   // Call-ID header
	fromURI    string   // From header URI
	toURI      string   // To header URI
	viaList    []string // Via headers (in order)
	cseq       string   // CSeq header
	sdp        *sdpInfo // Parsed SDP body (if Content-Type: application/sdp)
}

// parseSIPMessage parses SIP message headers and SDP body.
func (p *SIPParser) parseSIPMessage(payload []byte) (*sipMessage, error) {
	if len(payload) < 8 {
		return nil, fmt.Errorf("payload too short")
	}

	msg := &sipMessage{
		viaList: make([]string, 0, 2),
	}

	// Split headers and body by \r\n\r\n or \n\n
	headerEnd := bytes.Index(payload, []byte("\r\n\r\n"))
	if headerEnd == -1 {
		headerEnd = bytes.Index(payload, []byte("\n\n"))
		if headerEnd == -1 {
			headerEnd = len(payload) // No body
		}
	}

	headerData := payload[:headerEnd]
	lines := bytes.Split(headerData, []byte("\n"))

	// Parse first line (Request-Line or Status-Line)
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty message")
	}

	firstLine := string(bytes.TrimSpace(lines[0]))
	if strings.HasPrefix(firstLine, "SIP/2.0 ") {
		// Status-Line: SIP/2.0 200 OK
		parts := strings.SplitN(firstLine, " ", 3)
		if len(parts) >= 2 {
			code, _ := strconv.Atoi(parts[1])
			msg.statusCode = code
		}
	} else {
		// Request-Line: INVITE sip:bob@example.com SIP/2.0
		parts := strings.SplitN(firstLine, " ", 3)
		if len(parts) >= 1 {
			msg.method = parts[0]
		}
	}

	// Parse headers
	for i := 1; i < len(lines); i++ {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		// Header folding: lines starting with space/tab are continuations
		for i+1 < len(lines) && (lines[i+1][0] == ' ' || lines[i+1][0] == '\t') {
			i++
			line = append(line, ' ')
			line = append(line, bytes.TrimSpace(lines[i])...)
		}

		colonIdx := bytes.IndexByte(line, ':')
		if colonIdx == -1 {
			continue
		}

		name := string(bytes.TrimSpace(line[:colonIdx]))
		value := string(bytes.TrimSpace(line[colonIdx+1:]))

		// Parse key headers (case-insensitive)
		switch strings.ToLower(name) {
		case "call-id", "i":
			msg.callID = value
		case "from", "f":
			msg.fromURI = extractURI(value)
		case "to", "t":
			msg.toURI = extractURI(value)
		case "via", "v":
			msg.viaList = append(msg.viaList, value)
		case "cseq":
			msg.cseq = value
		}
	}

	// Parse SDP body if present
	bodyStart := headerEnd + 4 // skip \r\n\r\n
	if bodyStart < len(payload) {
		bodyData := payload[bodyStart:]
		if bytes.Contains(headerData, []byte("application/sdp")) {
			sdp, err := p.parseSDPBody(bodyData)
			if err == nil {
				msg.sdp = sdp
			}
		}
	}

	return msg, nil
}

// extractURI extracts URI from From/To header value.
// Example: "Alice" <sip:alice@example.com>;tag=1234 → sip:alice@example.com
func extractURI(value string) string {
	// Find <...> brackets
	start := strings.IndexByte(value, '<')
	if start == -1 {
		// No brackets, URI is the first token
		parts := strings.Fields(value)
		if len(parts) > 0 {
			// Remove trailing parameters (;xxx)
			uri := parts[0]
			if semiIdx := strings.IndexByte(uri, ';'); semiIdx != -1 {
				uri = uri[:semiIdx]
			}
			return uri
		}
		return ""
	}

	end := strings.IndexByte(value[start:], '>')
	if end == -1 {
		return ""
	}

	return value[start+1 : start+end]
}

// parseSDPBody parses SDP body (c=, m=, a= lines).
func (p *SIPParser) parseSDPBody(body []byte) (*sdpInfo, error) {
	sdp := &sdpInfo{
		mediaStreams: make([]mediaStream, 0, 2),
	}

	lines := bytes.Split(body, []byte("\n"))
	var sessionIP netip.Addr // Session-level c= line
	var currentMedia *mediaStream

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) < 2 || line[1] != '=' {
			continue
		}

		typ := line[0]
		value := string(bytes.TrimSpace(line[2:]))

		switch typ {
		case 'c':
			// c=IN IP4 192.168.1.100 or c=IN IP6 2001:db8::1
			ip := parseConnectionLine(value)
			if ip.IsValid() {
				if currentMedia != nil {
					// Media-level c= line
					sdp.connectionIP = ip
				} else {
					// Session-level c= line
					sessionIP = ip
				}
			}

		case 'm':
			// Save previous media stream
			if currentMedia != nil {
				sdp.mediaStreams = append(sdp.mediaStreams, *currentMedia)
			}

			// m=audio 49170 RTP/AVP 0 8
			// m=video 51372 RTP/AVP 31
			parts := strings.Fields(value)
			if len(parts) < 3 {
				continue
			}

			port, err := strconv.ParseUint(parts[1], 10, 16)
			if err != nil {
				continue
			}

			currentMedia = &mediaStream{
				mediaType: parts[0],
				rtpPort:   uint16(port),
				rtcpPort:  uint16(port) + 1, // Default RTCP port
				direction: "sendrecv",       // Default direction
				codec:     "",               // Will be set by first a=rtpmap
			}

		case 'a':
			if currentMedia == nil {
				continue // Session-level attribute, skip
			}

			// a=rtcp-mux
			if value == "rtcp-mux" {
				currentMedia.rtcpMux = true
				currentMedia.rtcpPort = currentMedia.rtpPort
				continue
			}

			// a=rtcp:53020
			if strings.HasPrefix(value, "rtcp:") {
				port, err := strconv.ParseUint(value[5:], 10, 16)
				if err == nil {
					currentMedia.rtcpPort = uint16(port)
				}
				continue
			}

			// a=rtpmap:0 PCMU/8000 (only save first codec)
			if strings.HasPrefix(value, "rtpmap:") {
				if currentMedia.codec == "" {
					parts := strings.SplitN(value[7:], " ", 2)
					if len(parts) == 2 {
						currentMedia.codec = parts[1]
					}
				}
				continue
			}

			// a=sendrecv / sendonly / recvonly / inactive
			if value == "sendrecv" || value == "sendonly" || value == "recvonly" || value == "inactive" {
				currentMedia.direction = value
			}
		}
	}

	// Save last media stream
	if currentMedia != nil {
		sdp.mediaStreams = append(sdp.mediaStreams, *currentMedia)
	}

	// Use session-level c= if no media-level c=
	if !sdp.connectionIP.IsValid() && sessionIP.IsValid() {
		sdp.connectionIP = sessionIP
	}

	if len(sdp.mediaStreams) == 0 {
		return nil, fmt.Errorf("no media streams in SDP")
	}

	return sdp, nil
}

// parseConnectionLine parses c= line and extracts IP address.
// c=IN IP4 192.168.1.100
// c=IN IP6 2001:db8::1
func parseConnectionLine(value string) netip.Addr {
	parts := strings.Fields(value)
	if len(parts) < 3 {
		return netip.Addr{}
	}

	// parts[0] = "IN", parts[1] = "IP4"/"IP6", parts[2] = IP address
	ip, err := netip.ParseAddr(parts[2])
	if err != nil {
		return netip.Addr{}
	}

	return ip
}

// handleSDP processes SDP based on SIP message type.
func (p *SIPParser) handleSDP(sipMsg *sipMessage, pkt *core.DecodedPacket) {
	if sipMsg.callID == "" {
		return
	}

	// Determine SIP message type
	isInvite := sipMsg.method == "INVITE"
	is200OK := sipMsg.statusCode == 200 && strings.Contains(sipMsg.cseq, "INVITE")
	isBye := sipMsg.method == "BYE"
	isCancel := sipMsg.method == "CANCEL"

	// Handle BYE/CANCEL (no SDP needed)
	if isBye || isCancel {
		p.cleanupFlows(sipMsg.callID)
		p.sessionCache.Delete(sipMsg.callID)
		return
	}

	// For INVITE and 200 OK, SDP is required
	if sipMsg.sdp == nil {
		return
	}

	switch {
	case isInvite:
		// Store offer SDP in session cache
		session := &sipSession{
			callID:    sipMsg.callID,
			offerSDP:  sipMsg.sdp,
			createdAt: time.Now(),
		}
		p.sessionCache.Set(sipMsg.callID, session, defaultSessionTTL)

	case is200OK:
		// Retrieve offer SDP and register bidirectional flows
		if cached, found := p.sessionCache.Get(sipMsg.callID); found {
			session := cached.(*sipSession)
			session.answerSDP = sipMsg.sdp

			// Register media flows
			p.registerMediaFlows(session, pkt)
		}
	}
}

// registerMediaFlows registers RTP/RTCP flows to FlowRegistry.
// Creates bidirectional FlowKeys for each media stream.
func (p *SIPParser) registerMediaFlows(session *sipSession, pkt *core.DecodedPacket) {
	if session.offerSDP == nil || session.answerSDP == nil {
		return
	}

	offerIP := session.offerSDP.connectionIP
	answerIP := session.answerSDP.connectionIP

	if !offerIP.IsValid() || !answerIP.IsValid() {
		return
	}

	// Match media streams by index (audio/video order should match)
	maxStreams := len(session.offerSDP.mediaStreams)
	if len(session.answerSDP.mediaStreams) < maxStreams {
		maxStreams = len(session.answerSDP.mediaStreams)
	}

	for i := 0; i < maxStreams; i++ {
		offerMedia := session.offerSDP.mediaStreams[i]
		answerMedia := session.answerSDP.mediaStreams[i]

		// Register RTP flows
		p.registerBidirectionalFlow(
			offerIP, answerIP,
			offerMedia.rtpPort, answerMedia.rtpPort,
			session.callID, offerMedia.codec,
		)

		// Register RTCP flows (if not muxed)
		if !offerMedia.rtcpMux && !answerMedia.rtcpMux {
			p.registerBidirectionalFlow(
				offerIP, answerIP,
				offerMedia.rtcpPort, answerMedia.rtcpPort,
				session.callID, "RTCP",
			)
		}
	}
}

// registerBidirectionalFlow registers two FlowKeys (A→B and B→A).
func (p *SIPParser) registerBidirectionalFlow(
	ipA, ipB netip.Addr,
	portA, portB uint16,
	callID, codec string,
) {
	flowContext := map[string]string{
		"call_id": callID,
		"codec":   codec,
	}

	// Flow A → B
	keyAtoB := plugin.FlowKey{
		SrcIP:   ipA,
		DstIP:   ipB,
		SrcPort: portA,
		DstPort: portB,
		Proto:   17, // UDP
	}
	p.flowRegistry.Set(keyAtoB, flowContext)

	// Flow B → A
	keyBtoA := plugin.FlowKey{
		SrcIP:   ipB,
		DstIP:   ipA,
		SrcPort: portB,
		DstPort: portA,
		Proto:   17, // UDP
	}
	p.flowRegistry.Set(keyBtoA, flowContext)
}

// cleanupFlows removes flows associated with a call from FlowRegistry.
func (p *SIPParser) cleanupFlows(callID string) {
	if p.flowRegistry == nil {
		return
	}

	// Iterate FlowRegistry and delete matching flows
	p.flowRegistry.Range(func(key plugin.FlowKey, value any) bool {
		if ctx, ok := value.(map[string]string); ok {
			if ctx["call_id"] == callID {
				p.flowRegistry.Delete(key)
			}
		}
		return true // continue iteration
	})
}
