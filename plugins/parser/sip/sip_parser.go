package sip

import (
	"bytes"
	"context"

	"firestige.xyz/otus/internal/config"
)

// SIP方法常量
var sipMethods = [][]byte{
	[]byte("INVITE"),
	[]byte("ACK"),
	[]byte("BYE"),
	[]byte("CANCEL"),
	[]byte("REGISTER"),
	[]byte("OPTIONS"),
	[]byte("PRACK"),
	[]byte("SUBSCRIBE"),
	[]byte("NOTIFY"),
	[]byte("PUBLISH"),
	[]byte("INFO"),
	[]byte("REFER"),
	[]byte("MESSAGE"),
	[]byte("UPDATE"),
}

// SIP版本标识，用于识别响应消息
var sipVersion = []byte("SIP/2.0")

type SipParser struct {
	config.CommonFields
	// TCP Assembly已经处理了缓冲和重组，这里不需要额外的buffer
}

// Detect 快速检测数据是否为SIP消息
// SIP请求以已知方法开头，SIP响应以"SIP/2.0"开头
func (p *SipParser) Detect(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	// 检查是否是SIP响应（以SIP/2.0开头）
	if bytes.HasPrefix(data, sipVersion) {
		return true
	}

	// 检查是否是SIP请求（以已知SIP方法开头）
	for _, method := range sipMethods {
		if bytes.HasPrefix(data, method) {
			// 确保方法后面跟着空格，避免误判
			if len(data) > len(method) && data[len(method)] == ' ' {
				return true
			}
		}
	}

	return false
}

// Extract 从数据中提取完整的SIP消息
// 由于TCP Assembly已经处理了流重组，这里直接处理完整的数据流
// SIP消息以双CRLF（\r\n\r\n）结束
func (p *SipParser) Extract(data []byte) (msg []byte, consumed int, err error) {
	if len(data) == 0 {
		return nil, 0, nil
	}

	// 查找消息结束标记（双CRLF）
	endMarker := []byte("\r\n\r\n")
	endIndex := bytes.Index(data, endMarker)

	if endIndex == -1 {
		// 没有找到完整消息，返回需要更多数据
		// TCP Assembly会继续提供更多数据
		return nil, 0, nil
	}

	// 找到完整消息，包含双CRLF
	messageEnd := endIndex + len(endMarker)
	message := make([]byte, messageEnd)
	copy(message, data[:messageEnd])

	return message, messageEnd, nil
}

// Reset 重置解析器状态
// 由于不使用内部缓冲，这里不需要做任何操作
func (p *SipParser) Reset() {
	// TCP Assembly处理缓冲，parser不需要维护状态
}

func (p *SipParser) Name() string {
	return "sip-parser"
}

func (p *SipParser) DefaultConfig() string {
	return ``
}

func (p *SipParser) PostConfig(ctx context.Context, cfg interface{}) error {
	return nil
}

func (p *SipParser) Start() error {
	return nil
}

func (p *SipParser) Close() error {
	return nil
}
