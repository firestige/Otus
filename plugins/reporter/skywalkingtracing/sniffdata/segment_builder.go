package sniffdata

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	common "skywalking.apache.org/repo/goapi/collect/common/v3"
	agent "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
	v1 "skywalking.apache.org/repo/goapi/satellite/data/v1"
)

type SegmentBuilder struct {
	ServiceName     string
	ServiceInstance string
	Timestamp       int64
	TraceId         string
	SegmentId       string
	Spans           []*agent.SpanObject // 包含的Span对象
}

func NewSegmentBuilder(serviceName string, instanceID string) *SegmentBuilder {
	segmentID := NewSegmentIDGenerator(instanceID).Generate()
	return &SegmentBuilder{
		ServiceName:     serviceName,
		ServiceInstance: instanceID,
		Spans:           make([]*agent.SpanObject, 0),
		SegmentId:       segmentID,
	}
}

func (b *SegmentBuilder) WithTimestamp(timestamp int64) *SegmentBuilder {
	// 设置时间戳
	b.Timestamp = timestamp
	return b
}

func (b *SegmentBuilder) WithTraceId(traceId string) *SegmentBuilder {
	// 设置跟踪ID
	b.TraceId = traceId
	return b
}

func (b *SegmentBuilder) WithSpan(span *agent.SpanObject) *SegmentBuilder {
	// 添加Span到段中
	b.Spans = append(b.Spans, span)
	return b
}

func (b *SegmentBuilder) WithSpans(spans []*agent.SpanObject) *SegmentBuilder {
	// 设置多个Span
	b.Spans = spans
	return b
}

func (b *SegmentBuilder) Build() *agent.SegmentObject {
	return &agent.SegmentObject{
		TraceId:         b.TraceId,
		TraceSegmentId:  b.SegmentId,
		Spans:           b.Spans,
		Service:         b.ServiceName,
		ServiceInstance: b.ServiceInstance,
		IsSizeLimited:   true,
	}
}

func WrapWithSniffData(segment *agent.SegmentObject) *v1.SniffData {
	startTime := segment.Spans[0].StartTime
	// 按照约定把TraceID加上SNIFFER-前缀
	if segment.TraceId != "" {
		segment.TraceId = "SNIFFER-" + segment.TraceId
	}
	//  如果span[0]的endtime<span[len-1].endtime，则使用span[len-1]的endtime替换span[0]的endtime
	if len(segment.Spans) > 0 && segment.Spans[0].EndTime < segment.Spans[len(segment.Spans)-1].EndTime {
		segment.Spans[0].EndTime = segment.Spans[len(segment.Spans)-1].EndTime
	}
	// 包装Segment为SniffData
	traceByte, _ := proto.Marshal(segment)
	return &v1.SniffData{
		Name:      "sip-capture",
		Timestamp: startTime,
		Type:      v1.SniffType_TracingType,
		Remote:    true,
		Data: &v1.SniffData_Segment{
			Segment: traceByte,
		},
	}
}

type SpanBuilder struct {
	SpanId        int32
	ParentSpanId  int32
	StartTime     int64
	EndTime       int64
	Refs          []*agent.SegmentReference
	OperationName string
	Peer          string
	SpanType      agent.SpanType
	SpanLayer     agent.SpanLayer
	ComponentId   int32
	IsError       bool
	Tags          []*common.KeyStringValuePair
	Logs          []*agent.Log
	SkipAnalysis  bool
	parent        *SegmentBuilder
}

func NewSpanBuilder() *SpanBuilder {
	return &SpanBuilder{
		SpanId:        0,
		ParentSpanId:  0,
		StartTime:     0,
		EndTime:       0,
		Refs:          make([]*agent.SegmentReference, 0),
		OperationName: "",
		Peer:          "",
		SpanType:      agent.SpanType_Local,
		SpanLayer:     agent.SpanLayer_Unknown,
		ComponentId:   0,
		IsError:       false,
		Tags:          make([]*common.KeyStringValuePair, 0),
		Logs:          make([]*agent.Log, 0),
		SkipAnalysis:  false,
	}
}

func (b *SpanBuilder) WithSpanId(spanId int32) *SpanBuilder {
	// 设置Span ID
	b.SpanId = spanId
	return b
}

func (b *SpanBuilder) WithParentSpanId(parentSpanId int32) *SpanBuilder {
	// 设置父Span ID
	b.ParentSpanId = parentSpanId
	return b
}

func (b *SpanBuilder) WithStartTime(startTime int64) *SpanBuilder {
	// 设置Span开始时间
	b.StartTime = startTime
	return b
}

func (b *SpanBuilder) WithEndTime(endTime int64) *SpanBuilder {
	// 设置Span结束时间
	b.EndTime = endTime
	return b
}

func (b *SpanBuilder) WithRef(refs *agent.SegmentReference) *SpanBuilder {
	// 设置Span引用
	b.Refs = append(b.Refs, refs)
	return b
}

func (b *SpanBuilder) WithOperationName(operationName string) *SpanBuilder {
	// 设置操作名称
	b.OperationName = operationName
	return b
}

func (b *SpanBuilder) WithPeer(peer string) *SpanBuilder {
	// 设置对端地址
	b.Peer = peer
	return b
}

func (b *SpanBuilder) WithSpanType(spanType agent.SpanType) *SpanBuilder {
	// 设置Span类型
	b.SpanType = spanType
	return b
}

func (b *SpanBuilder) WithSpanLayer(spanLayer agent.SpanLayer) *SpanBuilder {
	// 设置Span层级
	b.SpanLayer = spanLayer
	return b
}

func (b *SpanBuilder) WithComponentId(componentId int32) *SpanBuilder {
	// 设置组件ID
	b.ComponentId = componentId
	return b
}

func (b *SpanBuilder) WithIsError(isError bool) *SpanBuilder {
	// 设置是否为错误Span
	b.IsError = isError
	return b
}

func (b *SpanBuilder) WithTag(key string, value string) *SpanBuilder {
	// 设置单个标签
	b.Tags = append(b.Tags, &common.KeyStringValuePair{
		Key:   key,
		Value: value,
	})
	return b
}

func (b *SpanBuilder) WithTags(tags []*common.KeyStringValuePair) *SpanBuilder {
	// 设置标签
	b.Tags = tags
	return b
}

func (b *SpanBuilder) WithHeaders(headers map[string]string) *SpanBuilder {
	// 将Headers转换为标签
	for key, value := range headers {
		b.Tags = append(b.Tags, &common.KeyStringValuePair{
			Key:   key,
			Value: value,
		})
	}
	return b
}

func (b *SpanBuilder) WithLog(timestamp int64, metrices map[string]string) *SpanBuilder {
	// 设置单个日志
	log := &agent.Log{
		Time: timestamp,
		Data: make([]*common.KeyStringValuePair, 0),
	}
	for key, value := range metrices {
		log.Data = append(log.Data, &common.KeyStringValuePair{
			Key:   key,
			Value: value,
		})
	}
	b.Logs = append(b.Logs, log)
	return b
}

func (b *SpanBuilder) WithLogs(logs []*agent.Log) *SpanBuilder {
	// 设置日志
	b.Logs = logs
	return b
}

func (b *SpanBuilder) WithSkipAnalysis(skip bool) *SpanBuilder {
	// 设置是否跳过分析
	b.SkipAnalysis = skip
	return b
}

func (b *SpanBuilder) Build() *agent.SpanObject {
	// 构建Span对象
	return &agent.SpanObject{
		SpanId:        b.SpanId,
		ParentSpanId:  b.ParentSpanId,
		StartTime:     b.StartTime,
		EndTime:       b.EndTime,
		Refs:          b.Refs,
		OperationName: b.OperationName,
		Peer:          b.Peer,
		SpanType:      b.SpanType,
		SpanLayer:     b.SpanLayer,
		ComponentId:   b.ComponentId,
		IsError:       b.IsError,
		Tags:          b.Tags,
		Logs:          b.Logs,
		SkipAnalysis:  b.SkipAnalysis,
	}
}

type SegmentReferenceBuilder struct {
	TraceId                  string
	ParentTraceSegmentId     string
	ParentSpanId             int32
	ParentService            string
	ParentServiceInstance    string
	ParentEndpoint           string
	NetworkAddressUsedAtPeer string
}

func NewSegmentReferenceBuilder() *SegmentReferenceBuilder {
	return &SegmentReferenceBuilder{}
}

func (b *SegmentReferenceBuilder) WithTraceID(traceId string) *SegmentReferenceBuilder {
	// 设置TraceId
	b.TraceId = traceId
	return b
}

func (b *SegmentReferenceBuilder) WithParentTraceSegmentID(parentTraceSegmentId string) *SegmentReferenceBuilder {
	// 设置父TraceSegmentId
	b.ParentTraceSegmentId = parentTraceSegmentId
	return b
}

func (b *SegmentReferenceBuilder) WithParentSpanID(parentSpanId int32) *SegmentReferenceBuilder {
	// 设置父SpanId
	b.ParentSpanId = parentSpanId
	return b
}

func (b *SegmentReferenceBuilder) WithParentService(parentService string) *SegmentReferenceBuilder {
	// 设置父服务名称
	b.ParentService = parentService
	return b
}

func (b *SegmentReferenceBuilder) WithParentServiceInstance(parentServiceInstance string) *SegmentReferenceBuilder {
	// 设置父服务实例名称
	b.ParentServiceInstance = parentServiceInstance
	return b
}

func (b *SegmentReferenceBuilder) WithParentEndpoint(parentEndpoint string) *SegmentReferenceBuilder {
	// 设置父端点
	b.ParentEndpoint = parentEndpoint
	return b
}

func (b *SegmentReferenceBuilder) WithNetworkAddressUsedAtPeer(networkAddressUsedAtPeer string) *SegmentReferenceBuilder {
	// 设置网络地址
	b.NetworkAddressUsedAtPeer = networkAddressUsedAtPeer
	return b
}

func (b *SegmentReferenceBuilder) Build() *agent.SegmentReference {
	// 构建SegmentReference对象
	return &agent.SegmentReference{
		RefType:                  agent.RefType_CrossProcess,
		TraceId:                  b.TraceId,
		ParentTraceSegmentId:     b.ParentTraceSegmentId,
		ParentSpanId:             b.ParentSpanId,
		ParentService:            b.ParentService,
		ParentServiceInstance:    b.ParentServiceInstance,
		ParentEndpoint:           b.ParentEndpoint,
		NetworkAddressUsedAtPeer: b.NetworkAddressUsedAtPeer,
	}
}

type SegmentIDGenerator struct {
	instanceId string
	next       int64
	mutex      *sync.Mutex
}

func NewSegmentIDGenerator(instanceId string) *SegmentIDGenerator {
	return &SegmentIDGenerator{
		instanceId: instanceId,
		next:       0,
		mutex:      &sync.Mutex{},
	}
}

func (g *SegmentIDGenerator) Generate() string {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	// 获取当前goroutine ID (或使用固定ID)
	goroutineID := runtime.NumGoroutine() // 简化版本

	// 获取当前时间戳 (毫秒)
	timestamp := time.Now().UnixNano() / 1e6

	id := fmt.Sprintf("%s.%d.%d.%d",
		g.instanceId,
		goroutineID,
		timestamp,
		g.next)

	g.next++

	// 生成segment ID: instanceId.goroutineId.timestamp.sequence
	return id
}
