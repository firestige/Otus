package tracing

import (
	"firestige.xyz/otus/plugins/filter/skywalking/sniffdata"
	"firestige.xyz/otus/plugins/filter/skywalking/types"
	common "skywalking.apache.org/repo/goapi/collect/common/v3"
	agent "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

type TraceContext struct {
	traceID   string
	idMapping []string // 序号是span ID，内容是Dialog ID或者Transaction ID，特殊的，idMapping[0]是Call-ID
	segment   *agent.SegmentObject

	isInitalized bool // 是否已经初始化
}

func (ctx *TraceContext) addNewSpanWithDialog(dialog types.Dialog) {
	// 有且仅有 Invite 呼叫才会有Dialog，其他的都是Transaction。
	// 所以 Dialog 一定是 span[0]。
	// 根据 RFC3261，call-id+from-tag+to-tag 唯一标识一个 Dialog。
	// 需要 to-tag 的原因是在 fork 会话时，from-tag 一样，但是 to-tag 不一样。只使用早期会话的 call-id+from-tag 会导致冲突。
	// 但是我们的场景中没有 fork，多路呼叫通过 free switch 处理，所以这里不考虑 fork 场景。
	// 因此dialog ID 可以只使用 call-id+from-tag。
	span := sniffdata.NewSpanBuilder().SpanId(0).DialogId(dialog.ID()).Build()
	ctx.segment.Spans = append(ctx.segment.Spans, span)
}

func (ctx *TraceContext) endSpanWithDialog(dialog types.Dialog) {
	// 结束 Dialog 对应的 Span，
	// invite Dialog 一定会随着 transaction 的结束而结束，或者说 Dialog 结束后，transaction 也会结束。
	ctx.segment.Spans[0].EndTime = dialog.UpdatedAt()

}

func (ctx *TraceContext) endSpanWithDialogTimeout(transaction types.Dialog) {
	// 结束 Dialog 对应的 Span，一般而言 Dialog 是没有超时的，但是为了避免漏抓或者异常情况没有收到 bye 导致 Dialog 泄露，需要有一个超时机制释放内存
	span := ctx.segment.Spans[0]
	span.EndTime = transaction.TerminatedAt().UnixMilli()
	span.IsError = true
	span.Tags = append(span.Tags, &common.KeyStringValuePair{Key: "reason", Value: "timeout"})
}

func (ctx *TraceContext) addNewSpanWithTransaction(transaction types.Transaction) {
	span := sniffdata.NewSpanBuilder().SpanId(1).TransactionId(transaction.ID()).Build()
	ctx.segment.Spans = append(ctx.segment.Spans, span)
}

func (ctx *TraceContext) endSpanWithTransaction(transaction types.Transaction) {
	for i := range ctx.segment.Spans {
		if ctx.segment.Spans[i].TransactionId == transaction.ID() {
			ctx.segment.Spans[i].EndTime = transaction.TerminatedAt().UnixMilli()
			break
		}
	}
}

func (ctx *TraceContext) endSpanWithTransactionTimeout(transaction types.Transaction) {
	// 结束 Transaction 对应的 Span，
}
