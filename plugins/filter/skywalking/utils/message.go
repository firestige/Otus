package utils

import (
	"strings"

	"firestige.xyz/otus/plugins/filter/skywalking/types"
)

func ExtractURIAndTag(header string) (string, string) {
	// 示例输入: "sip:alice@example.com;tag=12345" 或 "<sip:alice@example.com>;tag=12345"
	uri := ""
	tag := ""

	// 去除尖括号
	h := header
	if start := len(h); start > 0 && h[0] == '<' {
		if end := len(h); end > 0 && h[end-1] == '>' {
			h = h[1 : end-1]
		}
	}

	// 查找分号，分割参数
	semi := -1
	for i := 0; i < len(h); i++ {
		if h[i] == ';' {
			semi = i
			break
		}
	}
	if semi == -1 {
		// 没有参数，直接返回
		return h, ""
	}
	uri = h[:semi]
	params := h[semi+1:]

	// 查找tag参数
	for _, param := range splitParams(params) {
		if len(param) >= 4 && param[:4] == "tag=" {
			tag = param[4:]
			break
		}
	}
	return uri, tag
}

// 辅助函数：分割参数字符串
func splitParams(params string) []string {
	var result []string
	start := 0
	for i := 0; i < len(params); i++ {
		if params[i] == ';' {
			if start < i {
				result = append(result, params[start:i])
			}
			start = i + 1
		}
	}
	if start < len(params) {
		result = append(result, params[start:])
	}
	return result
}

func ExtractMethodFromCseq(cseq string) types.Method {
	// CSeq格式通常为 "1 INVITE" 或 "2 ACK"
	parts := strings.SplitN(cseq, " ", 2)
	if len(parts) < 2 {
		return types.MethodUnknown
	}
	method := strings.ToUpper(parts[1])
	switch method {
	case "INVITE":
		return types.MethodInvite
	case "ACK":
		return types.MethodAck
	case "INFO":
		return types.MethodInfo
	case "BYE":
		return types.MethodBye
	case "CANCEL":
		return types.MethodCancel
	case "MESSAGE":
		return types.MethodMessage
	case "REFER":
		return types.MethodRefer
	case "PRACK":
		return types.MethodPrack
	case "UPDATE":
		return types.MethodUpdate
	case "OPTIONS":
		return types.MethodOptions
	case "REGISTER":
		return types.MethodRegister
	case "SUBSCRIBE":
		return types.MethodSubscribe
	case "NOTIFY":
		return types.MethodNotify
	default:
		return types.MethodUnknown
	}
}

func ParseUAType(msg types.SipMessage) types.UAType {
	// 利用msg的Direction和请求与响应来判断UA类型
	// 对UAS而言inbound收到来自外部的请求，outbound发送到外部的响应
	// 对UAC而言inbound收到来自外部的响应，outbound发送到外部的请求
	// 特殊的情况是in-dialog消息,主要是Info/Bye/CANCEL,无论UAC还是UAS都可以发起,但此时不应该创建会话
	if msg.Direction() == types.DirectionInbound {
		if msg.IsRequest() {
			return types.UAServer
		}
		return types.UAClient
	}
	if msg.Direction() == types.DirectionOutbound {
		if msg.IsRequest() {
			return types.UAClient
		}
		return types.UAServer
	}
	return types.UAUnknown
}

func IsProvisionalResponse(resp types.SipResponse) bool {
	status := resp.Status()
	return status >= 100 && status < 200
}

func IsFinalResponse(resp types.SipResponse) bool {
	status := resp.Status()
	return status >= 200 && status < 700
}

func Is2XXResponse(resp types.SipResponse) bool {
	status := resp.Status()
	return status >= 200 && status < 300
}

func IsNon2XXFinalResponse(resp types.SipResponse) bool {
	status := resp.Status()
	return status >= 300 && status < 700
}

func IsRedirectResponse(resp types.SipResponse) bool {
	status := resp.Status()
	return status >= 300 && status < 400
}

func IsErrorResponse(resp types.SipResponse) bool {
	status := resp.Status()
	return status >= 400 && status < 600
}

func GetBranchFromVia(via string) string {
	// 解析Via头部，提取branch参数
	if start := strings.Index(via, "branch="); start != -1 {
		start += len("branch=")
		end := strings.Index(via[start:], ";")
		if end == -1 {
			end = len(via)
		} else {
			end += start
		}
		return via[start:end]
	}
	return ""
}

func IsByeRequest(req types.SipRequest) bool {
	return req.Method() == types.MethodBye
}

func IsAckRequest(req types.SipRequest) bool {
	return req.Method() == types.MethodAck
}

func IsCancelRequest(req types.SipRequest) bool {
	return req.Method() == types.MethodCancel
}
