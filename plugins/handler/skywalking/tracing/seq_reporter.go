package tracing

import (
	"encoding/json"
	"fmt"
	"strings"

	"firestige.xyz/otus/plugins/handler/skywalking/types"
)

type SipSequenceData struct {
	RawMsg    string `json:"raw_msg"`
	IsRequest bool   `json:"is_request"`
	From      string `json:"from"`
	To        string `json:"to"`
	Local     string `json:"local"`
	Timestamp int64  `json:"timestamp"`
	RefID     string `json:"ref_id"` // 对话ID或事务ID
}

func NewSipSequenceData(msg types.SipMessage) *SipSequenceData {
	return &SipSequenceData{
		RawMsg:    msg.String(),
		IsRequest: msg.IsRequest(),
		From:      msg.SrcIP(),    // ip 可能要通过dns 解析获得
		To:        msg.DstIP(),    // ip
		Local:     msg.LocalURI(), // 本地地址"ip:port"
		Timestamp: msg.CreatedAt(),
		RefID:     fmt.Sprintf("%s/%s", msg.CallID(), strings.ReplaceAll(msg.CSeq(), " ", "_")),
	}
}

func (data *SipSequenceData) String() string {
	// 转 json
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Sprintf("SipSequenceData{error: %v}", err)
	}
	return string(jsonBytes)
}
