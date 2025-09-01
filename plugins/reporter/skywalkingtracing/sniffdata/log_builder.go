package sniffdata

import (
	"google.golang.org/protobuf/proto"
	common "skywalking.apache.org/repo/goapi/collect/common/v3"
	logging "skywalking.apache.org/repo/goapi/collect/logging/v3"
	v1 "skywalking.apache.org/repo/goapi/satellite/data/v1"
)

type LogBuilder struct {
	ServiceName     string
	ServiceInstance string
	Timestamp       int64
	Endpoint        string
	Body            string
	Tags            *logging.LogTags
	TraceId         string
	SegmentId       string
	SpanId          int32
}

func NewLogBuilder() *LogBuilder {
	return &LogBuilder{}
}

func (b *LogBuilder) WithServiceName(serviceName string) *LogBuilder {
	// 设置服务名称
	b.ServiceName = serviceName
	return b
}

func (b *LogBuilder) WithServiceInstance(serviceInstance string) *LogBuilder {
	// 设置服务实例名称
	b.ServiceInstance = serviceInstance
	return b
}

func (b *LogBuilder) WithTimestamp(timestamp int64) *LogBuilder {
	// 设置时间戳
	b.Timestamp = timestamp
	return b
}

func (b *LogBuilder) WithEndpoint(endpoint string) *LogBuilder {
	// 设置端点
	b.Endpoint = endpoint
	return b
}

func (b *LogBuilder) WithBody(body string) *LogBuilder {
	// 设置日志体
	b.Body = body
	return b
}

func (b *LogBuilder) WithTag(key string, value string) *LogBuilder {
	// 设置单个标签
	if b.Tags == nil {
		b.Tags = &logging.LogTags{
			Data: make([]*common.KeyStringValuePair, 0),
		}
		b.Tags.Data = append(b.Tags.Data, &common.KeyStringValuePair{
			Key:   key,
			Value: value,
		})
	}
	return b
}

func (b *LogBuilder) WithTags(tags map[string]string) *LogBuilder {
	// 设置多个标签
	if b.Tags == nil {
		b.Tags = &logging.LogTags{
			Data: make([]*common.KeyStringValuePair, 0, len(tags)),
		}
	}
	for key, value := range tags {
		b.Tags.Data = append(b.Tags.Data, &common.KeyStringValuePair{
			Key:   key,
			Value: value,
		})
	}
	return b
}

func (b *LogBuilder) WithTraceContext(traceId, segmentId string, spanId int32) *LogBuilder {
	// 设置追踪上下文
	b.TraceId = traceId
	b.SegmentId = segmentId
	b.SpanId = spanId
	return b
}

func (b *LogBuilder) buildLogData() *logging.LogData {
	return &logging.LogData{
		Service:         b.ServiceName,
		ServiceInstance: b.ServiceInstance,
		Timestamp:       b.Timestamp,
		Endpoint:        b.Endpoint,
		Body: &logging.LogDataBody{
			Type: "LogDataBodyType_TEXT",
			Content: &logging.LogDataBody_Text{
				Text: &logging.TextLog{
					Text: string(b.Body),
				},
			},
		},
		Tags: b.Tags,
		TraceContext: &logging.TraceContext{
			TraceId:        b.TraceId,
			TraceSegmentId: b.SegmentId,
			SpanId:         b.SpanId,
		},
	}
}

func (b *LogBuilder) Build() *v1.SniffData {
	packet := b.buildLogData()
	packetByte, _ := proto.Marshal(packet)
	return &v1.SniffData{
		Name:      "sip-log",
		Timestamp: b.Timestamp,
		Type:      v1.SniffType_Logging,
		Remote:    true,
		Data: &v1.SniffData_LogList{
			LogList: &v1.BatchLogList{
				Logs: [][]byte{packetByte},
			},
		},
	}
}
