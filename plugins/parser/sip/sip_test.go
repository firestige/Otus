package sip

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"firestige.xyz/otus/internal/core"
	"firestige.xyz/otus/pkg/plugin"
)

// mockFlowRegistry implements plugin.FlowRegistry for testing.
type mockFlowRegistry struct {
	flows map[plugin.FlowKey]any
}

func newMockFlowRegistry() *mockFlowRegistry {
	return &mockFlowRegistry{
		flows: make(map[plugin.FlowKey]any),
	}
}

func (m *mockFlowRegistry) Get(key plugin.FlowKey) (any, bool) {
	v, ok := m.flows[key]
	return v, ok
}

func (m *mockFlowRegistry) Set(key plugin.FlowKey, value any) {
	m.flows[key] = value
}

func (m *mockFlowRegistry) Delete(key plugin.FlowKey) {
	delete(m.flows, key)
}

func (m *mockFlowRegistry) Range(f func(key plugin.FlowKey, value any) bool) {
	for k, v := range m.flows {
		if !f(k, v) {
			break
		}
	}
}

func (m *mockFlowRegistry) Count() int {
	return len(m.flows)
}

func (m *mockFlowRegistry) Clear() {
	m.flows = make(map[plugin.FlowKey]any)
}

func TestCanHandle(t *testing.T) {
	parser := NewSIPParser().(*SIPParser)

	tests := []struct {
		name     string
		pkt      *core.DecodedPacket
		expected bool
	}{
		{
			name: "port 5060 src",
			pkt: &core.DecodedPacket{
				Transport: core.TransportHeader{SrcPort: 5060},
			},
			expected: true,
		},
		{
			name: "port 5060 dst",
			pkt: &core.DecodedPacket{
				Transport: core.TransportHeader{DstPort: 5060},
			},
			expected: true,
		},
		{
			name: "port 5061",
			pkt: &core.DecodedPacket{
				Transport: core.TransportHeader{DstPort: 5061},
			},
			expected: true,
		},
		{
			name: "INVITE magic",
			pkt: &core.DecodedPacket{
				Transport: core.TransportHeader{DstPort: 5070}, // non-standard port
				Payload:   []byte("INVITE sip:bob@example.com SIP/2.0\r\n"),
			},
			expected: true,
		},
		{
			name: "SIP/2.0 response magic",
			pkt: &core.DecodedPacket{
				Transport: core.TransportHeader{DstPort: 5070},
				Payload:   []byte("SIP/2.0 200 OK\r\n"),
			},
			expected: true,
		},
		{
			name: "REGISTER magic",
			pkt: &core.DecodedPacket{
				Payload: []byte("REGISTER sip:example.com SIP/2.0\r\n"),
			},
			expected: true,
		},
		{
			name: "not SIP",
			pkt: &core.DecodedPacket{
				Transport: core.TransportHeader{DstPort: 8080},
				Payload:   []byte("HTTP/1.1 200 OK\r\n"),
			},
			expected: false,
		},
		{
			name: "too short",
			pkt: &core.DecodedPacket{
				Payload: []byte("SIP"),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.CanHandle(tt.pkt)
			if result != tt.expected {
				t.Errorf("CanHandle() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestExtractURI(t *testing.T) {
	tests := []struct {
		value    string
		expected string
	}{
		{
			value:    `"Alice" <sip:alice@example.com>;tag=1234`,
			expected: "sip:alice@example.com",
		},
		{
			value:    "<sip:bob@192.168.1.1:5060>",
			expected: "sip:bob@192.168.1.1:5060",
		},
		{
			value:    "sip:carol@example.com",
			expected: "sip:carol@example.com",
		},
		{
			value:    "sip:dave@example.com;transport=udp",
			expected: "sip:dave@example.com",
		},
		{
			value:    "<>",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := extractURI(tt.value)
			if result != tt.expected {
				t.Errorf("extractURI(%q) = %q, expected %q", tt.value, result, tt.expected)
			}
		})
	}
}

func TestParseSIPMessage(t *testing.T) {
	parser := NewSIPParser().(*SIPParser)

	t.Run("INVITE request", func(t *testing.T) {
		payload := []byte("INVITE sip:bob@example.com SIP/2.0\r\n" +
			"Via: SIP/2.0/UDP 192.168.1.100:5060;branch=z9hG4bK776asdhds\r\n" +
			"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
			"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
			"To: Bob <sip:bob@biloxi.com>\r\n" +
			"CSeq: 314159 INVITE\r\n" +
			"\r\n")

		msg, err := parser.parseSIPMessage(payload)
		if err != nil {
			t.Fatalf("parseSIPMessage failed: %v", err)
		}

		if msg.method != "INVITE" {
			t.Errorf("method = %q, expected INVITE", msg.method)
		}
		if msg.callID != "a84b4c76e66710@pc33.atlanta.com" {
			t.Errorf("callID = %q", msg.callID)
		}
		if msg.fromURI != "sip:alice@atlanta.com" {
			t.Errorf("fromURI = %q", msg.fromURI)
		}
		if msg.toURI != "sip:bob@biloxi.com" {
			t.Errorf("toURI = %q", msg.toURI)
		}
		if len(msg.viaList) != 1 {
			t.Errorf("len(viaList) = %d, expected 1", len(msg.viaList))
		}
	})

	t.Run("200 OK response", func(t *testing.T) {
		payload := []byte("SIP/2.0 200 OK\r\n" +
			"Via: SIP/2.0/UDP 192.168.1.100:5060;branch=z9hG4bK776asdhds\r\n" +
			"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
			"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
			"To: Bob <sip:bob@biloxi.com>;tag=a6c85cf\r\n" +
			"CSeq: 314159 INVITE\r\n" +
			"\r\n")

		msg, err := parser.parseSIPMessage(payload)
		if err != nil {
			t.Fatalf("parseSIPMessage failed: %v", err)
		}

		if msg.statusCode != 200 {
			t.Errorf("statusCode = %d, expected 200", msg.statusCode)
		}
		if msg.method != "" {
			t.Errorf("method should be empty for response, got %q", msg.method)
		}
		if !strings.Contains(msg.cseq, "INVITE") {
			t.Errorf("cseq = %q, should contain INVITE", msg.cseq)
		}
	})
}

func TestParseSDPBody(t *testing.T) {
	parser := NewSIPParser().(*SIPParser)

	t.Run("basic audio SDP", func(t *testing.T) {
		sdpBody := []byte("v=0\r\n" +
			"o=alice 2890844526 2890844526 IN IP4 192.168.1.100\r\n" +
			"s=Session\r\n" +
			"c=IN IP4 192.168.1.100\r\n" +
			"t=0 0\r\n" +
			"m=audio 49170 RTP/AVP 0 8\r\n" +
			"a=rtpmap:0 PCMU/8000\r\n" +
			"a=rtpmap:8 PCMA/8000\r\n")

		sdp, err := parser.parseSDPBody(sdpBody)
		if err != nil {
			t.Fatalf("parseSDPBody failed: %v", err)
		}

		expectedIP := netip.MustParseAddr("192.168.1.100")
		if sdp.connectionIP != expectedIP {
			t.Errorf("connectionIP = %v, expected %v", sdp.connectionIP, expectedIP)
		}

		if len(sdp.mediaStreams) != 1 {
			t.Fatalf("len(mediaStreams) = %d, expected 1", len(sdp.mediaStreams))
		}

		media := sdp.mediaStreams[0]
		if media.mediaType != "audio" {
			t.Errorf("mediaType = %q, expected audio", media.mediaType)
		}
		if media.rtpPort != 49170 {
			t.Errorf("rtpPort = %d, expected 49170", media.rtpPort)
		}
		if media.rtcpPort != 49171 {
			t.Errorf("rtcpPort = %d, expected 49171 (default)", media.rtcpPort)
		}
		if media.rtcpMux {
			t.Error("rtcpMux should be false")
		}
		if media.codec != "PCMU/8000" {
			t.Errorf("codec = %q, expected PCMU/8000", media.codec)
		}
	})

	t.Run("RTCP-MUX", func(t *testing.T) {
		sdpBody := []byte("v=0\r\n" +
			"c=IN IP4 10.0.0.1\r\n" +
			"m=audio 50000 RTP/AVP 0\r\n" +
			"a=rtcp-mux\r\n" +
			"a=rtpmap:0 PCMU/8000\r\n")

		sdp, err := parser.parseSDPBody(sdpBody)
		if err != nil {
			t.Fatalf("parseSDPBody failed: %v", err)
		}

		media := sdp.mediaStreams[0]
		if !media.rtcpMux {
			t.Error("rtcpMux should be true")
		}
		if media.rtcpPort != media.rtpPort {
			t.Errorf("rtcpPort = %d, should equal rtpPort %d when muxed", media.rtcpPort, media.rtpPort)
		}
	})

	t.Run("explicit RTCP port", func(t *testing.T) {
		sdpBody := []byte("v=0\r\n" +
			"c=IN IP4 10.0.0.1\r\n" +
			"m=audio 50000 RTP/AVP 0\r\n" +
			"a=rtcp:50001\r\n" +
			"a=rtpmap:0 PCMU/8000\r\n")

		sdp, err := parser.parseSDPBody(sdpBody)
		if err != nil {
			t.Fatalf("parseSDPBody failed: %v", err)
		}

		media := sdp.mediaStreams[0]
		if media.rtcpPort != 50001 {
			t.Errorf("rtcpPort = %d, expected 50001", media.rtcpPort)
		}
	})

	t.Run("multiple media streams", func(t *testing.T) {
		sdpBody := []byte("v=0\r\n" +
			"c=IN IP4 10.0.0.1\r\n" +
			"m=audio 50000 RTP/AVP 0\r\n" +
			"a=rtpmap:0 PCMU/8000\r\n" +
			"m=video 50002 RTP/AVP 31\r\n" +
			"a=rtpmap:31 H261/90000\r\n")

		sdp, err := parser.parseSDPBody(sdpBody)
		if err != nil {
			t.Fatalf("parseSDPBody failed: %v", err)
		}

		if len(sdp.mediaStreams) != 2 {
			t.Fatalf("len(mediaStreams) = %d, expected 2", len(sdp.mediaStreams))
		}

		if sdp.mediaStreams[0].mediaType != "audio" {
			t.Error("first stream should be audio")
		}
		if sdp.mediaStreams[1].mediaType != "video" {
			t.Error("second stream should be video")
		}
	})
}

func TestHandleINVITEAndResponse(t *testing.T) {
	parser := NewSIPParser().(*SIPParser)
	registry := newMockFlowRegistry()
	parser.SetFlowRegistry(registry)

	// INVITE with SDP offer
	invitePayload := []byte("INVITE sip:bob@example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 192.168.1.100:5060\r\n" +
		"Call-ID: test-call-123@example.com\r\n" +
		"From: <sip:alice@example.com>;tag=1\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Type: application/sdp\r\n" +
		"\r\n" +
		"v=0\r\n" +
		"o=alice 2890844526 2890844526 IN IP4 192.168.1.100\r\n" +
		"s=Session\r\n" +
		"c=IN IP4 192.168.1.100\r\n" +
		"t=0 0\r\n" +
		"m=audio 30000 RTP/AVP 0\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n")

	invitePkt := &core.DecodedPacket{
		Transport: core.TransportHeader{SrcPort: 5060, DstPort: 5060},
		Payload:   invitePayload,
	}

	// Handle INVITE
	_, labels, err := parser.Handle(invitePkt)
	if err != nil {
		t.Fatalf("Handle INVITE failed: %v", err)
	}

	if labels[core.LabelSIPMethod] != "INVITE" {
		t.Errorf("method label = %q, expected INVITE", labels[core.LabelSIPMethod])
	}
	if labels[core.LabelSIPCallID] != "test-call-123@example.com" {
		t.Errorf("call-id label = %q", labels[core.LabelSIPCallID])
	}

	// At this point, session should be cached but no flows registered yet
	if registry.Count() != 0 {
		t.Errorf("FlowRegistry count = %d after INVITE, expected 0", registry.Count())
	}

	// 200 OK with SDP answer
	responsePayload := []byte("SIP/2.0 200 OK\r\n" +
		"Via: SIP/2.0/UDP 192.168.1.100:5060\r\n" +
		"Call-ID: test-call-123@example.com\r\n" +
		"From: <sip:alice@example.com>;tag=1\r\n" +
		"To: <sip:bob@example.com>;tag=2\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Type: application/sdp\r\n" +
		"\r\n" +
		"v=0\r\n" +
		"o=bob 2890844527 2890844527 IN IP4 192.168.1.200\r\n" +
		"s=Session\r\n" +
		"c=IN IP4 192.168.1.200\r\n" +
		"t=0 0\r\n" +
		"m=audio 40000 RTP/AVP 0\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n")

	responsePkt := &core.DecodedPacket{
		Transport: core.TransportHeader{SrcPort: 5060, DstPort: 5060},
		Payload:   responsePayload,
	}

	// Handle 200 OK
	_, labels, err = parser.Handle(responsePkt)
	if err != nil {
		t.Fatalf("Handle 200 OK failed: %v", err)
	}

	if labels[core.LabelSIPStatusCode] != "200" {
		t.Errorf("status code label = %q, expected 200", labels[core.LabelSIPStatusCode])
	}

	// Now flows should be registered (bidirectional: 2 RTP + 2 RTCP = 4 flows)
	if registry.Count() != 4 {
		t.Errorf("FlowRegistry count = %d after 200 OK, expected 4 (2 RTP + 2 RTCP bidirectional)", registry.Count())
	}

	// Verify flow keys
	aliceIP := netip.MustParseAddr("192.168.1.100")
	bobIP := netip.MustParseAddr("192.168.1.200")

	// Alice → Bob RTP
	keyAtoB_RTP := plugin.FlowKey{
		SrcIP:   aliceIP,
		DstIP:   bobIP,
		SrcPort: 30000,
		DstPort: 40000,
		Proto:   17,
	}
	if _, ok := registry.Get(keyAtoB_RTP); !ok {
		t.Error("Alice → Bob RTP flow not registered")
	}

	// Bob → Alice RTP
	keyBtoA_RTP := plugin.FlowKey{
		SrcIP:   bobIP,
		DstIP:   aliceIP,
		SrcPort: 40000,
		DstPort: 30000,
		Proto:   17,
	}
	if _, ok := registry.Get(keyBtoA_RTP); !ok {
		t.Error("Bob → Alice RTP flow not registered")
	}
}

func TestHandleBYE(t *testing.T) {
	parser := NewSIPParser().(*SIPParser)
	registry := newMockFlowRegistry()
	parser.SetFlowRegistry(registry)

	// Setup: INVITE + 200 OK to create flows
	invitePayload := []byte("INVITE sip:bob@example.com SIP/2.0\r\n" +
		"Call-ID: bye-test-call@example.com\r\n" +
		"From: <sip:alice@example.com>\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Type: application/sdp\r\n" +
		"\r\n" +
		"v=0\r\nc=IN IP4 10.0.0.1\r\nt=0 0\r\nm=audio 20000 RTP/AVP 0\r\n")

	invitePkt := &core.DecodedPacket{Payload: invitePayload, Transport: core.TransportHeader{DstPort: 5060}}
	parser.Handle(invitePkt)

	responsePayload := []byte("SIP/2.0 200 OK\r\n" +
		"Call-ID: bye-test-call@example.com\r\n" +
		"From: <sip:alice@example.com>\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Type: application/sdp\r\n" +
		"\r\n" +
		"v=0\r\nc=IN IP4 10.0.0.2\r\nt=0 0\r\nm=audio 30000 RTP/AVP 0\r\n")

	responsePkt := &core.DecodedPacket{Payload: responsePayload, Transport: core.TransportHeader{DstPort: 5060}}
	parser.Handle(responsePkt)

	if registry.Count() == 0 {
		t.Fatal("No flows registered after INVITE/200 OK")
	}

	// Send BYE
	byePayload := []byte("BYE sip:bob@example.com SIP/2.0\r\n" +
		"Call-ID: bye-test-call@example.com\r\n" +
		"From: <sip:alice@example.com>\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"CSeq: 2 BYE\r\n" +
		"\r\n")

	byePkt := &core.DecodedPacket{Payload: byePayload, Transport: core.TransportHeader{DstPort: 5060}}
	_, labels, err := parser.Handle(byePkt)
	if err != nil {
		t.Fatalf("Handle BYE failed: %v", err)
	}

	if labels[core.LabelSIPMethod] != "BYE" {
		t.Errorf("method label = %q, expected BYE", labels[core.LabelSIPMethod])
	}

	// Flows should be cleaned up
	if registry.Count() != 0 {
		t.Errorf("FlowRegistry count = %d after BYE, expected 0", registry.Count())
	}
}

func TestPluginLifecycle(t *testing.T) {
	parser := NewSIPParser().(*SIPParser)

	if parser.Name() != "sip" {
		t.Errorf("Name() = %q, expected sip", parser.Name())
	}

	if err := parser.Init(nil); err != nil {
		t.Errorf("Init failed: %v", err)
	}

	ctx := context.Background()
	if err := parser.Start(ctx); err != nil {
		t.Errorf("Start failed: %v", err)
	}

	// Simulate adding data to session cache
	parser.sessionCache.Set("test-key", "test-value", time.Hour)
	if parser.sessionCache.ItemCount() != 1 {
		t.Error("session cache should have 1 item")
	}

	// Stop should flush cache
	if err := parser.Stop(ctx); err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	if parser.sessionCache.ItemCount() != 0 {
		t.Error("session cache should be empty after Stop")
	}
}

func TestMultiChannelMediaStreams(t *testing.T) {
	parser := NewSIPParser().(*SIPParser)
	registry := newMockFlowRegistry()
	parser.SetFlowRegistry(registry)

	// INVITE with 2 audio + 1 video (3 media streams)
	invitePayload := []byte("INVITE sip:bob@example.com SIP/2.0\r\n" +
		"Call-ID: multi-channel-test@example.com\r\n" +
		"From: <sip:alice@example.com>\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Type: application/sdp\r\n" +
		"\r\n" +
		"v=0\r\n" +
		"o=alice 2890844526 2890844526 IN IP4 192.168.1.100\r\n" +
		"s=Multi-channel Session\r\n" +
		"c=IN IP4 192.168.1.100\r\n" +
		"t=0 0\r\n" +
		"m=audio 20000 RTP/AVP 0\r\n" + // Audio channel 1
		"a=rtpmap:0 PCMU/8000\r\n" +
		"m=audio 20002 RTP/AVP 8\r\n" + // Audio channel 2
		"a=rtpmap:8 PCMA/8000\r\n" +
		"m=video 30000 RTP/AVP 31\r\n" + // Video channel
		"a=rtpmap:31 H261/90000\r\n")

	invitePkt := &core.DecodedPacket{
		Transport: core.TransportHeader{DstPort: 5060},
		Payload:   invitePayload,
	}

	_, _, err := parser.Handle(invitePkt)
	if err != nil {
		t.Fatalf("Handle INVITE failed: %v", err)
	}

	// 200 OK with matching 3 media streams
	responsePayload := []byte("SIP/2.0 200 OK\r\n" +
		"Call-ID: multi-channel-test@example.com\r\n" +
		"From: <sip:alice@example.com>\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Type: application/sdp\r\n" +
		"\r\n" +
		"v=0\r\n" +
		"o=bob 2890844527 2890844527 IN IP4 192.168.1.200\r\n" +
		"s=Multi-channel Session\r\n" +
		"c=IN IP4 192.168.1.200\r\n" +
		"t=0 0\r\n" +
		"m=audio 40000 RTP/AVP 0\r\n" + // Audio channel 1
		"a=rtpmap:0 PCMU/8000\r\n" +
		"m=audio 40002 RTP/AVP 8\r\n" + // Audio channel 2
		"a=rtpmap:8 PCMA/8000\r\n" +
		"m=video 50000 RTP/AVP 31\r\n" + // Video channel
		"a=rtpmap:31 H261/90000\r\n")

	responsePkt := &core.DecodedPacket{
		Transport: core.TransportHeader{DstPort: 5060},
		Payload:   responsePayload,
	}

	_, _, err = parser.Handle(responsePkt)
	if err != nil {
		t.Fatalf("Handle 200 OK failed: %v", err)
	}

	// Verify all 3 media streams registered flows
	// 3 media streams × (2 RTP + 2 RTCP) = 12 flows
	expectedFlows := 12
	if registry.Count() != expectedFlows {
		t.Errorf("FlowRegistry count = %d, expected %d (3 media streams × 4 flows each)",
			registry.Count(), expectedFlows)
	}

	// Verify specific flow keys exist
	aliceIP := netip.MustParseAddr("192.168.1.100")
	bobIP := netip.MustParseAddr("192.168.1.200")

	testCases := []struct {
		name    string
		srcIP   netip.Addr
		dstIP   netip.Addr
		srcPort uint16
		dstPort uint16
	}{
		{"Audio1 RTP Alice→Bob", aliceIP, bobIP, 20000, 40000},
		{"Audio1 RTP Bob→Alice", bobIP, aliceIP, 40000, 20000},
		{"Audio1 RTCP Alice→Bob", aliceIP, bobIP, 20001, 40001},
		{"Audio1 RTCP Bob→Alice", bobIP, aliceIP, 40001, 20001},
		{"Audio2 RTP Alice→Bob", aliceIP, bobIP, 20002, 40002},
		{"Audio2 RTP Bob→Alice", bobIP, aliceIP, 40002, 20002},
		{"Video RTP Alice→Bob", aliceIP, bobIP, 30000, 50000},
		{"Video RTP Bob→Alice", bobIP, aliceIP, 50000, 30000},
	}

	for _, tc := range testCases {
		key := plugin.FlowKey{
			SrcIP:   tc.srcIP,
			DstIP:   tc.dstIP,
			SrcPort: tc.srcPort,
			DstPort: tc.dstPort,
			Proto:   17,
		}
		if _, ok := registry.Get(key); !ok {
			t.Errorf("Flow not registered: %s (%v:%d → %v:%d)",
				tc.name, tc.srcIP, tc.srcPort, tc.dstIP, tc.dstPort)
		}
	}
}

func BenchmarkCanHandle(b *testing.B) {
	parser := NewSIPParser().(*SIPParser)
	pkt := &core.DecodedPacket{
		Transport: core.TransportHeader{DstPort: 5060},
		Payload:   []byte("INVITE sip:test@example.com SIP/2.0\r\n"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.CanHandle(pkt)
	}
}

func BenchmarkHandleSIPMessage(b *testing.B) {
	parser := NewSIPParser().(*SIPParser)
	registry := newMockFlowRegistry()
	parser.SetFlowRegistry(registry)

	payload := []byte("INVITE sip:bob@example.com SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 192.168.1.100:5060\r\n" +
		"Call-ID: bench-call@example.com\r\n" +
		"From: <sip:alice@example.com>;tag=1\r\n" +
		"To: <sip:bob@example.com>\r\n" +
		"CSeq: 1 INVITE\r\n" +
		"Content-Type: application/sdp\r\n" +
		"\r\n" +
		"v=0\r\nc=IN IP4 192.168.1.100\r\nt=0 0\r\nm=audio 30000 RTP/AVP 0\r\n")

	pkt := &core.DecodedPacket{
		Transport: core.TransportHeader{DstPort: 5060},
		Payload:   payload,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Handle(pkt)
	}
}
