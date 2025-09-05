package tracing

import (
	"fmt"
	"strings"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/handler/skywalking/sniffdata"
	"firestige.xyz/otus/plugins/handler/skywalking/types"
	"firestige.xyz/otus/plugins/handler/skywalking/utils"
	common "skywalking.apache.org/repo/goapi/collect/common/v3"
	agent "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	v1 "skywalking.apache.org/repo/goapi/satellite/data/v1"
)

type TraceContext struct {
	traceID   string
	idMapping []string // 序号是span ID，内容是Dialog ID或者Transaction ID，特殊的，idMapping[0]是Call-ID
	segment   *agent.SegmentObject

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

func (ctx *TraceContext) addNewSpanWithDialog(dialog types.Dialog) {
	// 有且仅有 Invite 呼叫才会有Dialog，其他的都是Transaction。
	// 所以 Dialog 一定是 span[0]。
	// 根据 RFC3261，call-id+from-tag+to-tag 唯一标识一个 Dialog。
	// 需要 to-tag 的原因是在 fork 会话时，from-tag 一样，但是 to-tag 不一样。只使用早期会话的 call-id+from-tag 会导致冲突。
	// 但是我们的场景中没有 fork，多路呼叫通过 free switch 处理，所以这里不考虑 fork 场景。
	// 因此dialog ID 可以只使用 call-id+from-tag。
	// 此外，由于抓包场景下无法获取 ref，所以根节点的ref只能是空字符串。
	span := sniffdata.
		NewSpanBuilder().
		WithSpanId(0).
		WithStartTime(dialog.CreatedAt()).
		WithOperationName(strings.ToUpper(dialog.MethodAsString())).
		WithSpanType(agent.SpanType_Entry).
		WithSpanLayer(agent.SpanLayer_Unknown).                 // 自定义场景在protobuf中未定义，统统为unknown
		WithPeer(utils.ExtractHostFromURI(dialog.RemoteURI())). // 使用remoteURI作为对端地址
		WithComponentId(0).                                     // 自定义场景在protobuf中未定义，统统为0
		WithTags()
	Build()
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

func (ctx *TraceContext) initSegmentObject(req types.SipRequest, uaType types.UAType) {
	ctx.segment.Spans = make([]*agent.SpanObject, 0)
	ctx.idMapping = make([]string, 0)
	switch uaType {
	case types.UAClient:
		remoteURI, _ := utils.ExtractURIAndTag(req.To())
		ctx.buildRootSpan(req.CallID(), req.MethodAsString(), remoteURI, req.CreatedAt())
	case types.UAServer:
		remoteURI, _ := utils.ExtractURIAndTag(req.From())
		// 对于服务器端请求，使用From作为对端地址
		ctx.buildRootSpan(req.CallID(), req.MethodAsString(), remoteURI, req.CreatedAt())
	default:
		// 未知UA类型，无法初始化Segment
		err := fmt.Errorf("unknown UA type: %v", uaType)
		log.GetLogger().WithError(err).Debugf("failed to inital segment: %v", uaType)
	}
	ctx.isInitalized = true
}

func (ctx *TraceContext) CreateNewSpan(id, parent, method, remoteURI string, startTime int64, headers map[string]string) {
	if !ctx.isInitalized {
		// 快速失败，没有初始化的TraceContext无法创建新的Span
		log.GetLogger().Errorf("TraceContext not initialized, cannot create new span for ID: %s", id)
		return
	}
	spanID := len(ctx.idMapping)
	parentID := ctx.getParentSpanID(parent)
	ctx.idMapping = append(ctx.idMapping, id)
	// 创建新的Span
	span := sniffdata.NewSpanBuilder().
		WithSpanId(int32(spanID)).
		WithParentSpanId(parentID).
		WithStartTime(startTime).
		WithOperationName(strings.ToUpper(method)).
		WithHeaders(headers).
		WithSpanType(agent.SpanType_Local).     //为了方便管理先全部设置为Local
		WithSpanLayer(agent.SpanLayer_Unknown). // 自定义场景在protobuf中未定义，统统为unknown
		WithPeer(remoteURI).                    // 使用remoteURI作为对端地址
		Build()
	ctx.segment.Spans = append(ctx.segment.Spans, span)
}

func (ctx *TraceContext) FinishExistSpan(id string, isError bool, endTime int64) {
	for i, record := range ctx.idMapping {
		if record == id {
			span := ctx.segment.Spans[i]
			span.EndTime = endTime
			span.IsError = isError
			log.GetLogger().Infof("Finished span with ID %s in trace context %s, from: %d to %d", id, ctx.traceID, span.StartTime, span.EndTime)
			return
		}
	}
	// TODO 讨论是不是预定义ErrNotFound然后用log.GetLogger().WithError(ErrNotFound).Errorf()比较好
	log.GetLogger().Errorf("span with ID %s not found in trace context %s", id, ctx.traceID)
}

// 根Span是一个虚拟的Span，通常用于表示当前服务抓包的起点，主要用于解决in-dialog对话时出现多个fork dialog的情况，以及fs桥接会话时出现多条腿的情况。
func (ctx *TraceContext) buildRootSpan(callID string, method string, remoteURI string, startTime int64) {
	// 外呼场景FS作为下游只有上游组件获取traceID，所以根节点的ref肯定不为空
	ref := sniffdata.NewSegmentReferenceBuilder().
		WithTraceID(ctx.traceID).
		WithParentTraceSegmentID("").  //  TODO 上下文暂时不支持，用空字符串替代，由OAP修改
		WithParentSpanID(0).           //  TODO 上下文暂时不支持，用0替代，让OAP修改。记得打通ESL之后结合ESL的上下文修改
		WithParentService("").         //  TODO 上下文暂时不支持，用空字符串替代，由OAP修改
		WithParentServiceInstance(""). //  TODO 上下文暂时不支持，用空字符串替代，由OAP修改
		WithParentEndpoint("").        //  TODO 上下文暂时不支持，用空字符串替代，由OAP修改
		Build()
	// 记录callID到idMapping中，方便后续查找
	ctx.idMapping = append(ctx.idMapping, callID)
	span := sniffdata.NewSpanBuilder().
		WithSpanId(0).        // 根span的ID通常为0
		WithParentSpanId(-1). // 根span没有父span
		WithStartTime(startTime).
		WithOperationName(strings.ToUpper(method)).
		WithSpanType(agent.SpanType_Entry).
		WithSpanLayer(agent.SpanLayer_Unknown). // 自定义场景在protobuf中未定义，统统为unknown
		WithPeer(remoteURI).                    // 使用remoteURI作为对端地址
		WithRef(ref).
		Build()
	ctx.segment.Spans = append(ctx.segment.Spans, span)
}

// 使用Dialog ID从idMapping中寻找parentID
// 由于sip对话（dialog）> 事务（transaction），且事务不能嵌套事务，所以这里只有dialog可能为parent，
// 也可能没有dialogID，此时对应out-dialog会话，parent固定为0
func (ctx *TraceContext) getParentSpanID(id string) int32 {
	for i, record := range ctx.idMapping {
		if record == id {
			return int32(i)
		}
	}
	return int32(0)
}

func (ctx *TraceContext) sendSegment(channel chan *v1.SniffData) {
	if ctx.isInitalized && len(ctx.segment.Spans) > 1 {
		// 发送SegmentObject到channel
		data := sniffdata.WrapWithSniffData(ctx.segment)
		channel <- data
	} else {
		log.GetLogger().Warnf("TraceContext %s is not initialized or has no spans, skipping send", ctx.traceID)
	}
}
