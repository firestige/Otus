package sip

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

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
// 严格按照SIP协议解析：先找到头部结束位置，然后根据Content-Length提取完整消息体
func (p *SipParser) Extract(data []byte) (msg []byte, consumed int, err error) {
	if len(data) == 0 {
		return nil, 0, nil
	}

	// 查找头部结束标记（双CRLF）
	headerEndMarker := []byte("\r\n\r\n")
	headerEndIndex := bytes.Index(data, headerEndMarker)

	if headerEndIndex == -1 {
		// 没有找到头部结束标记，需要更多数据
		return nil, 0, nil
	}

	// 头部结束位置（不包含双CRLF）
	headerEnd := headerEndIndex
	bodyStart := headerEndIndex + len(headerEndMarker)

	// 解析头部，查找Content-Length
	headerData := data[:headerEnd]
	contentLength, err := p.parseContentLength(headerData)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid SIP message: %w", err)
	}

	// 计算完整消息长度
	totalMessageLength := bodyStart + contentLength

	// 检查是否有足够的数据
	if len(data) < totalMessageLength {
		// 数据不完整，需要更多数据
		return nil, 0, nil
	}

	// 提取完整消息
	message := make([]byte, totalMessageLength)
	copy(message, data[:totalMessageLength])

	return message, totalMessageLength, nil
}

// parseContentLength 解析SIP头部中的Content-Length字段
func (p *SipParser) parseContentLength(headerData []byte) (int, error) {
	headerStr := string(headerData)

	// SIP头部是大小写不敏感的
	lines := strings.Split(headerStr, "\r\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lineLower := strings.ToLower(line)

		// 检查Content-Length头部（支持缩写形式 l:）
		if strings.HasPrefix(lineLower, "content-length:") {
			return p.extractContentLengthValue(line, "content-length:")
		} else if strings.HasPrefix(lineLower, "l:") {
			return p.extractContentLengthValue(line, "l:")
		}
	}

	// 如果没有找到Content-Length头部，默认为0（没有消息体）
	return 0, nil
}

// extractContentLengthValue 提取Content-Length的值
func (p *SipParser) extractContentLengthValue(line, prefix string) (int, error) {
	// 安全地检查长度
	if len(line) <= len(prefix) {
		return 0, fmt.Errorf("invalid Content-Length header: %s", line)
	}

	// 使用大小写不敏感的查找来定位分隔符的位置
	colonIndex := strings.Index(strings.ToLower(line), strings.ToLower(prefix))
	if colonIndex == -1 {
		return 0, fmt.Errorf("Content-Length prefix not found: %s", line)
	}

	// 提取冒号后的值
	valueStart := colonIndex + len(prefix)
	if valueStart >= len(line) {
		return 0, fmt.Errorf("no value found for Content-Length: %s", line)
	}

	valueStr := strings.TrimSpace(line[valueStart:])
	if valueStr == "" {
		return 0, fmt.Errorf("empty Content-Length value: %s", line)
	}

	// 解析数字
	contentLength, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, fmt.Errorf("invalid Content-Length value: %s", valueStr)
	}

	if contentLength < 0 {
		return 0, fmt.Errorf("negative Content-Length value: %d", contentLength)
	}

	return contentLength, nil
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
