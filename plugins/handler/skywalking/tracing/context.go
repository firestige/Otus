package tracing

import (
	"slices"
	"strings"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/handler/skywalking/sniffdata"
	"firestige.xyz/otus/plugins/handler/skywalking/types"
	agent "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

type TraceContext struct {
	traceID      string
	idMapping    []string // 序号是span ID，内容是Dialog ID或者Transaction ID，特殊的，idMapping[0]是Call-ID
	segment      *agent.SegmentObject
	spanBuilders []*sniffdata.SpanBuilder

	isInitalized bool // 是否已经初始化
}

func (ctx *TraceContext) canBeReleased() bool {
	if ctx.isInitalized && len(ctx.segment.Spans) > 0 {
		// 检查所有 Span 是否都已结束
		for _, span := range ctx.segment.Spans {
			if span.EndTime == 0 {
				return false
			}
		}
		return true
	}
	return false
}

// createBaseSpan 创建基础的 Span Builder，包含通用配置
func (ctx *TraceContext) createBaseSpan(meta types.WithValue) *sniffdata.SpanBuilder {
	spanId := int32(len(ctx.spanBuilders)) // 新增的 spanId 是当前已有 span 的数量
	return sniffdata.
		NewSpanBuilder().
		WithSpanId(spanId).
		WithStartTime(meta.ValueAsInt64("start_time")).
		WithOperationName(strings.ToUpper(meta.ValueAsString("operation_name"))).
		WithSpanLayer(agent.SpanLayer_Unknown).     // 自定义场景在protobuf中未定义，统统为unknown
		WithPeer(meta.ValueAsString("remote_url")). // 使用remoteURI作为对端地址
		WithComponentId(0)                          // 自定义场景在protobuf中未定义，统统为0
}

// addSpanToContext 将 Span 添加到上下文中
func (ctx *TraceContext) addSpanToContext(span *sniffdata.SpanBuilder, id string, prepend bool) {
	if prepend {
		// Dialog ID 必须在最前面
		ctx.idMapping = append([]string{id}, ctx.idMapping...)
		ctx.spanBuilders = append([]*sniffdata.SpanBuilder{span}, ctx.spanBuilders...)
	} else {
		// Transaction ID 追加到末尾
		ctx.idMapping = append(ctx.idMapping, id)
		ctx.spanBuilders = append(ctx.spanBuilders, span)
	}
}

func (ctx *TraceContext) addNewSpanWithDialog(dialog types.Dialog) {
	// 有且仅有 Invite 呼叫才会有Dialog，其他的都是Transaction。
	// 所以 Dialog 必须是 span[0]。
	// 根据 RFC3261，call-id+from-tag+to-tag 唯一标识一个 Dialog。
	// 需要 to-tag 的原因是在 fork 会话时，from-tag 一样，但是 to-tag 不一样。只使用早期会话的 call-id+from-tag 会导致冲突。
	// 但是我们的场景中没有 fork，多路呼叫通过 free switch 处理，所以这里不考虑 fork 场景。
	// 因此dialog ID 可以只使用 call-id+from-tag。
	// 此外，由于抓包场景下无法获取 ref，所以根节点的ref只能是空字符串。

	if len(ctx.spanBuilders) > 0 {
		log.GetLogger().Warnf("TraceContext %s already has a dialog span, replacing it", ctx.traceID)
		ctx.spanBuilders = make([]*sniffdata.SpanBuilder, 0)
		ctx.idMapping = make([]string, 0)
	}

	span := ctx.createBaseSpan(dialog.(types.WithValue)).
		WithSpanType(agent.SpanType_Entry).
		WithTag("SIP-CALL-ID", dialog.(types.WithValue).ValueAsString("call_id")).
		WithTag("X-ICC-CALL-ID", dialog.(types.WithValue).ValueAsString("x_icc_call_id"))

	ctx.addSpanToContext(span, dialog.ID(), true)
}

// endSpanByIndex 根据索引结束 Span
func (ctx *TraceContext) endSpanByIndex(index int, endTime int64, isError bool, reason string) {
	if index < 0 || index >= len(ctx.spanBuilders) {
		log.GetLogger().Warnf("Invalid span index %d in trace context %s", index, ctx.traceID)
		return
	}

	spanBuilder := ctx.spanBuilders[index]
	spanBuilder.WithEndTime(endTime)

	if isError {
		spanBuilder.WithIsError(true)
		if reason != "" {
			spanBuilder.WithTag("reason", reason)
		}
	}
}

// findSpanIndex 根据 ID 查找 Span 的索引
func (ctx *TraceContext) findSpanIndex(id string) int {
	return slices.Index(ctx.idMapping, id)
}

func (ctx *TraceContext) endSpanWithDialog(dialog types.Dialog) {
	// 结束 Dialog 对应的 Span，
	// invite Dialog 一定会随着 transaction 的结束而结束，或者说 Dialog 结束后，transaction 也会结束。
	if len(ctx.spanBuilders) < 1 {
		log.GetLogger().Warnf("No spans in trace context %s to end for dialog %s", ctx.traceID, dialog.ID())
		return
	}
	ctx.endSpanByIndex(0, dialog.UpdatedAt(), false, "")
}

func (ctx *TraceContext) endSpanWithDialogTimeout(dialog types.Dialog) {
	// 结束 Dialog 对应的 Span，一般而言 Dialog 是没有超时的，但是为了避免漏抓或者异常情况没有收到 bye 导致 Dialog 泄露，需要有一个超时机制释放内存
	if len(ctx.spanBuilders) < 1 {
		log.GetLogger().Warnf("No spans in trace context %s to end for dialog %s", ctx.traceID, dialog.ID())
		return
	}
	ctx.endSpanByIndex(0, dialog.UpdatedAt(), true, "timeout")
}

func (ctx *TraceContext) addNewSpanWithTransaction(transaction types.Transaction) {
	// 如果这是个非Invite的Transaction，那么它没有Dialog，必须创建一个新的span
	// 简单说来，会话内消息（in-dialog）都有Dialog，且Dialog是span[0]，事务（transaction）是span[1..N]。
	// 会话外消息（out-dialog）没有Dialog，事务（transaction）是span[0]，因为事务对应的是一个 req 和 结束resp。
	// 另外，由于抓包场景下无法获取 ref，所以 transaction 的 ref 只能是空字符串。
	// 由于sip对话（dialog）> 事务（transaction），且事务不能嵌套事务，所以这里的parent只能是dialog ID，或者没有parent（out-dialog场景）
	seq_data := NewSipSequenceData(transaction.Request())
	spans := len(ctx.spanBuilders)

	span := ctx.createBaseSpan(transaction.(types.WithValue)).
		WithSpanType(agent.SpanType_Local). // 为了方便管理先全部设置为Local
		WithTag("sip_seq_data", seq_data.String())

	// 如果已有 spans，设置父 span ID
	if spans > 0 {
		span.WithParentSpanId(int32(spans - 1))
	}

	ctx.addSpanToContext(span, transaction.ID(), false)
}

func (ctx *TraceContext) endSpanWithTransaction(transaction types.Transaction) {
	if len(ctx.spanBuilders) < 1 {
		log.GetLogger().Warnf("No spans in trace context %s to end for transaction %s", ctx.traceID, transaction.ID())
		return
	}

	idx := ctx.findSpanIndex(transaction.ID())
	if idx < 0 {
		log.GetLogger().Warnf("Span for transaction %s not found in trace context %s", transaction.ID(), ctx.traceID)
		return
	}

	ctx.endSpanByIndex(idx, transaction.UpdatedAt(), false, "")
}

func (ctx *TraceContext) endSpanWithTransactionTimeout(transaction types.Transaction) {
	// 结束 Transaction 对应的 Span，
	if len(ctx.spanBuilders) < 1 {
		log.GetLogger().Warnf("No spans in trace context %s to end for transaction %s", ctx.traceID, transaction.ID())
		return
	}

	idx := ctx.findSpanIndex(transaction.ID())
	if idx < 0 {
		log.GetLogger().Warnf("Span for transaction %s not found in trace context %s", transaction.ID(), ctx.traceID)
		return
	}

	ctx.endSpanByIndex(idx, transaction.UpdatedAt(), true, "timeout")
}
