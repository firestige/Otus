package tracing

import (
	"fmt"
	"strings"
	"sync"

	"firestige.xyz/otus/internal/log"
	"firestige.xyz/otus/plugins/filter/skywalking/sniffdata"
	"firestige.xyz/otus/plugins/filter/skywalking/types"
	"firestige.xyz/otus/plugins/filter/skywalking/utils"
	agent "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	v1 "skywalking.apache.org/repo/goapi/satellite/data/v1"
)

type TraceManager struct {
	serviceName       string
	serviceInstanceId string
	traceContext      *sync.Map // key: trace ID
	mappings          *sync.Map // key: call-id, value: trace ID
}

func NewTraceManager(serviceName, serviceInstanceId string) *TraceManager {
	return &TraceManager{
		serviceName:       serviceName,
		serviceInstanceId: serviceInstanceId,
		traceContext:      &sync.Map{},
		mappings:          &sync.Map{},
	}
}

func (m *TraceManager) GetTraceContextByTraceID(traceID string) (*TraceContext, bool) {
	ctx, exists := m.traceContext.Load(traceID)
	if !exists {
		return nil, false
	}
	return ctx.(*TraceContext), true
}

func (m *TraceManager) GetTraceContextByCallID(callID string) (*TraceContext, bool) {
	if traceID, exists := m.mappings.Load(callID); exists {
		return m.GetTraceContextByTraceID(traceID.(string))
	}
	return nil, false
}

func (m *TraceManager) CreateTraceContext(traceID string, createAt int64) *TraceContext {
	if _, exists := m.traceContext.Load(traceID); !exists {
		// 创建新的TraceContext
		segment := sniffdata.NewSegmentBuilder(m.serviceName, m.serviceInstanceId).
			WithTraceId(traceID).
			WithTimestamp(createAt).
			Build()
		ctx := &TraceContext{
			traceID:      traceID,
			segment:      segment,
			idMapping:    make([]string, 0), // 初始化idMapping
			isInitalized: false,             // 初始状态为未初始化
		}
		m.traceContext.Store(traceID, ctx)
	}

	ctx, ok := m.traceContext.Load(traceID)
	if !ok {
		return nil
	}
	return ctx.(*TraceContext)
}

func (m *TraceManager) AliasWithCallID(traceID string, callID string) error {
	if _, exists := m.GetTraceContextByTraceID(traceID); exists {
		m.mappings.Store(callID, traceID)
		return nil
	}
	return fmt.Errorf("trace context not found for trace ID %s", traceID)
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
