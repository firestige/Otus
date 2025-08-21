package sip

import (
	"testing"
)

func TestSipParser_Detect(t *testing.T) {
	parser := &SipParser{}

	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "SIP INVITE request",
			data:     []byte("INVITE sip:alice@example.com SIP/2.0\r\n"),
			expected: true,
		},
		{
			name:     "SIP REGISTER request",
			data:     []byte("REGISTER sip:example.com SIP/2.0\r\n"),
			expected: true,
		},
		{
			name:     "SIP 200 OK response",
			data:     []byte("SIP/2.0 200 OK\r\n"),
			expected: true,
		},
		{
			name:     "SIP 404 Not Found response",
			data:     []byte("SIP/2.0 404 Not Found\r\n"),
			expected: true,
		},
		{
			name:     "Not SIP - HTTP request",
			data:     []byte("GET /index.html HTTP/1.1\r\n"),
			expected: false,
		},
		{
			name:     "Not SIP - random data",
			data:     []byte("some random data"),
			expected: false,
		},
		{
			name:     "Empty data",
			data:     []byte(""),
			expected: false,
		},
		{
			name:     "False positive - INVITE without space",
			data:     []byte("INVITEsomething"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.Detect(tt.data)
			if result != tt.expected {
				t.Errorf("Detect() = %v, expected %v for data: %s", result, tt.expected, string(tt.data))
			}
		})
	}
}

func TestSipParser_Extract(t *testing.T) {
	parser := &SipParser{}

	tests := []struct {
		name             string
		input            []byte
		expectedMsg      []byte
		expectedConsumed int
		expectError      bool
	}{
		{
			name: "Complete SIP message",
			input: []byte("INVITE sip:alice@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP client.example.com:5060\r\n" +
				"From: Bob <sip:bob@example.com>\r\n" +
				"To: Alice <sip:alice@example.com>\r\n" +
				"\r\n"),
			expectedMsg: []byte("INVITE sip:alice@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP client.example.com:5060\r\n" +
				"From: Bob <sip:bob@example.com>\r\n" +
				"To: Alice <sip:alice@example.com>\r\n" +
				"\r\n"),
			expectedConsumed: 150, // 整个消息的长度
			expectError:      false,
		},
		{
			name: "Incomplete SIP message",
			input: []byte("INVITE sip:alice@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP client.example.com:5060\r\n"),
			expectedMsg:      nil,
			expectedConsumed: 0, // 没有找到完整消息，消费0字节
			expectError:      false,
		},
		{
			name: "SIP response message",
			input: []byte("SIP/2.0 200 OK\r\n" +
				"Via: SIP/2.0/UDP server.example.com:5060\r\n" +
				"From: Bob <sip:bob@example.com>\r\n" +
				"\r\n"),
			expectedMsg: []byte("SIP/2.0 200 OK\r\n" +
				"Via: SIP/2.0/UDP server.example.com:5060\r\n" +
				"From: Bob <sip:bob@example.com>\r\n" +
				"\r\n"),
			expectedConsumed: 93,
			expectError:      false,
		},
		{
			name: "Multiple SIP messages in one data block",
			input: []byte("INVITE sip:alice@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP client.example.com:5060\r\n" +
				"\r\n" +
				"ACK sip:alice@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP client.example.com:5060\r\n" +
				"\r\n"),
			expectedMsg: []byte("INVITE sip:alice@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP client.example.com:5060\r\n" +
				"\r\n"),
			expectedConsumed: 82, // 第一条消息的长度
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser.Reset() // 重置解析器状态
			msg, consumed, err := parser.Extract(tt.input)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if consumed != tt.expectedConsumed {
				t.Errorf("Expected consumed %d, got %d", tt.expectedConsumed, consumed)
			}
			if string(msg) != string(tt.expectedMsg) {
				t.Errorf("Expected message:\n%s\nGot:\n%s", string(tt.expectedMsg), string(msg))
			}
		})
	}
}

func TestSipParser_Reset(t *testing.T) {
	parser := &SipParser{}

	// 由于不再使用内部缓冲区，Reset方法主要是为了接口兼容
	// 这个测试只验证Reset方法不会panic或产生错误
	parser.Reset()

	// 验证Reset后依然可以正常工作
	data := []byte("INVITE sip:alice@example.com SIP/2.0\r\n\r\n")
	detected := parser.Detect(data)
	if !detected {
		t.Error("Expected parser to still work after Reset()")
	}

	msg, consumed, err := parser.Extract(data)
	if err != nil {
		t.Errorf("Unexpected error after Reset(): %v", err)
	}
	if consumed != len(data) {
		t.Errorf("Expected to consume all data (%d), got %d", len(data), consumed)
	}
	if msg == nil {
		t.Error("Expected to extract message after Reset()")
	}
}
