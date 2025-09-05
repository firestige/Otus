package tracing

import (
	"fmt"
	"strings"
	"sync"

	"firestige.xyz/otus/plugins/handler/skywalking/sniffdata"
)

var TRACE_PREFIX = "SNIFFER-"

type TraceManager struct {
	serviceName       string
	serviceInstanceId string
	traceContext      *sync.Map // key: trace ID
}

func NewTraceManager(serviceName, serviceInstanceId string) *TraceManager {
	return &TraceManager{
		serviceName:       serviceName,
		serviceInstanceId: serviceInstanceId,
		traceContext:      &sync.Map{},
	}
}

// GetTraceContextByRefID 根据引用ID获取追踪上下文
// refID 可以是以下三种类型之一：
//  1. call-id: 用于标识 SIP 消息的唯一标识符
//  2. dialogID: 格式为 "call-id+from-tag"，用于标识 SIP 对话
//     生成方法请参考 utils.BuildDialogID
//  3. transactionID: 格式为 "call-id+cesq+branch"，用于标识 SIP 事务
//     生成方法请参考 utils.BuildTransactionID
//
// 参数:
//
//	refID - 引用标识符，可以是对话ID或事务ID
//
// 返回值:
//
//	*TraceContext - 找到的追踪上下文，如果不存在则为nil
//	bool - 是否找到对应的追踪上下文
//
// 另请参见: utils.BuildDialogID, utils.BuildTransactionID
func (m *TraceManager) GetTraceContextByRefID(id string) (*TraceContext, bool) {
	traceId := m.resolveTraceIDFromRefId(id)
	ctx, exists := m.traceContext.Load(traceId)
	if !exists {
		return nil, false
	}
	return ctx.(*TraceContext), true
}

func (m *TraceManager) CreateTraceContext(traceID string, createAt int64) *TraceContext {
	traceID = m.addSnifferPrefix(traceID)
	ctx, exists := m.traceContext.Load(traceID)
	if !exists {
		// 创建新的TraceContext
		segment := sniffdata.NewSegmentBuilder(m.serviceName, m.serviceInstanceId).
			WithTraceId(traceID).
			WithTimestamp(createAt).
			Build()
		ctx = &TraceContext{
			traceID:      traceID,
			segment:      segment,
			idMapping:    make([]string, 0), // 初始化idMapping
			isInitalized: false,             // 初始状态为未初始化
		}
		m.traceContext.Store(traceID, ctx)
	}

	return ctx.(*TraceContext)
}

func (m *TraceManager) resolveTraceIDFromRefId(refID string) string {
	// 查找第一个 "|" 的位置
	index := strings.Index(refID, "|")
	if index == -1 {
		// 如果没有找到 "|"，原样返回
		return refID
	}
	// 返回第一个 "|" 之前的字符串
	return m.addSnifferPrefix(refID[:index])
}

func (m *TraceManager) addSnifferPrefix(callID string) string {
	if strings.HasPrefix(callID, TRACE_PREFIX) {
		return callID
	}
	return fmt.Sprintf("%s%s", TRACE_PREFIX, callID)
}

func (m *TraceManager) ReleaseTraceContext(ctx *TraceContext) {
	m.traceContext.Delete(ctx.traceID)
}
