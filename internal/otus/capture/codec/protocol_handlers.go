package codec

import "bytes"

// SIPProtocolHandler SIP协议处理器
type SIPProtocolHandler struct{}

func (h *SIPProtocolHandler) CanHandle(transportProto byte, port uint16, payload []byte) bool {
	return len(payload) > 16 && (bytes.Contains(payload[:32], []byte("SIP")) ||
		bytes.Contains(payload[:32], []byte("INVITE")))
}

func (h *SIPProtocolHandler) Handle(msg *NetworkMessage) (*ProtocolResult, error) {
	// 简化的SIP处理
	return &ProtocolResult{
		MessageType:      1,
		ProcessedContent: msg.Content,
		SessionID:        []byte("sip-session-id"),
	}, nil
}

func (h *SIPProtocolHandler) GetType() byte {
	return 1
}

// RTPProtocolHandler RTP协议处理器
type RTPProtocolHandler struct{}

func (h *RTPProtocolHandler) CanHandle(transportProto byte, port uint16, payload []byte) bool {
	return len(payload) >= 2 && (payload[0]&0xC0) == 0x80 && port%2 == 0
}

func (h *RTPProtocolHandler) Handle(msg *NetworkMessage) (*ProtocolResult, error) {
	return &ProtocolResult{
		MessageType:      2,
		ProcessedContent: msg.Content,
	}, nil
}

func (h *RTPProtocolHandler) GetType() byte {
	return 2
}

// RTCPProtocolHandler RTCP协议处理器
type RTCPProtocolHandler struct{}

func (h *RTCPProtocolHandler) CanHandle(transportProto byte, port uint16, payload []byte) bool {
	return len(payload) >= 2 && (payload[0]&0xC0) == 0x80 && port%2 != 0
}

func (h *RTCPProtocolHandler) Handle(msg *NetworkMessage) (*ProtocolResult, error) {
	return &ProtocolResult{
		MessageType:      5,
		ProcessedContent: msg.Content,
		SessionID:        []byte("rtcp-session"),
	}, nil
}

func (h *RTCPProtocolHandler) GetType() byte {
	return 5
}
