package codec

import (
	"bytes"

	"github.com/google/gopacket/layers"
)

// ApplicationProcessor 应用层处理器
type ApplicationProcessor struct {
	protocolHandlers map[byte]ProtocolHandler
}

// NewApplicationProcessor 创建应用层处理器
func NewApplicationProcessor() *ApplicationProcessor {
	ap := &ApplicationProcessor{
		protocolHandlers: make(map[byte]ProtocolHandler),
	}

	// 注册协议处理器
	ap.RegisterHandler(&SIPProtocolHandler{})
	ap.RegisterHandler(&RTPProtocolHandler{})
	ap.RegisterHandler(&RTCPProtocolHandler{})

	return ap
}

// RegisterHandler 注册协议处理器
func (ap *ApplicationProcessor) RegisterHandler(handler ProtocolHandler) {
	ap.protocolHandlers[handler.GetType()] = handler
}

// ProcessMessage 处理传输层消息
func (ap *ApplicationProcessor) ProcessMessage(msg *NetworkMessage) (*NetworkMessage, error) {
	// 检查RTP/RTCP
	if msg.TransportProto == uint8(layers.IPProtocolUDP) {
		if ap.isRTPRTCPMessage(msg) {
			return ap.processRTPRTCPMessage(msg)
		}
	}

	// 尝试应用层协议处理
	return ap.processApplicationMessage(msg)
}

// isRTPRTCPMessage 检查是否为RTP/RTCP消息
func (ap *ApplicationProcessor) isRTPRTCPMessage(msg *NetworkMessage) bool {
	if len(msg.Content) < 2 {
		return false
	}

	// 检查RTP/RTCP版本号
	version := (msg.Content[0] & 0xC0) >> 6
	if version != 2 {
		return false
	}

	// 根据端口号判断
	if msg.SourcePort%2 == 0 && msg.DestinationPort%2 == 0 {
		return true // 可能是RTP
	}

	if msg.SourcePort%2 != 0 && msg.DestinationPort%2 != 0 {
		// 检查RTCP包类型
		packetType := msg.Content[1]
		return packetType == 200 || packetType == 201 || packetType == 207
	}

	return false
}

// processRTPRTCPMessage 处理RTP/RTCP消息
func (ap *ApplicationProcessor) processRTPRTCPMessage(msg *NetworkMessage) (*NetworkMessage, error) {
	if msg.SourcePort%2 != 0 && msg.DestinationPort%2 != 0 {
		// RTCP消息
		msg.ApplicationType = 5 // RTCP

		if handler, exists := ap.protocolHandlers[5]; exists {
			result, err := handler.Handle(msg)
			if err == nil && result != nil {
				msg.ApplicationType = result.MessageType
				msg.CallID = result.SessionID
			}
		}
	} else {
		// RTP消息
		msg.ApplicationType = 2 // RTP

		if handler, exists := ap.protocolHandlers[2]; exists {
			_, _ = handler.Handle(msg)
		}
		return nil, nil // RTP包通常不需要发送到输出队列
	}

	return msg, nil
}

// processApplicationMessage 处理应用层消息
func (ap *ApplicationProcessor) processApplicationMessage(msg *NetworkMessage) (*NetworkMessage, error) {
	// 尝试SIP处理
	if ap.isSIPMessage(msg.Content) {
		msg.ApplicationType = 1 // SIP

		if handler, exists := ap.protocolHandlers[1]; exists {
			result, err := handler.Handle(msg)
			if err == nil && result != nil {
				msg.ApplicationType = result.MessageType
				msg.CallID = result.SessionID
			}
		}

		return msg, nil
	}

	return nil, nil // 未知协议不处理
}

// isSIPMessage 检查是否为SIP消息
func (ap *ApplicationProcessor) isSIPMessage(content []byte) bool {
	if len(content) < 16 {
		return false
	}

	// 检查SIP请求方法
	sipMethods := [][]byte{
		[]byte("INVITE"), []byte("REGISTER"), []byte("ACK"),
		[]byte("BYE"), []byte("CANCEL"), []byte("OPTIONS"),
	}

	searchLen := 32
	if len(content) < searchLen {
		searchLen = len(content)
	}

	for _, method := range sipMethods {
		if bytes.Contains(content[:searchLen], method) {
			return true
		}
	}

	// 检查SIP响应
	return bytes.Contains(content[:searchLen], []byte("SIP/2.0"))
}
