package eventbus

import (
	"context"
)

// Event 事件结构
type Event struct {
	Topic  string      `json:"topic"`
	CallID string      `json:"call_id"`
	Ctx    interface{} `json:"ctx"`
}

// Handler 事件处理器
type Handler func(event *Event) error

// Subscriber 订阅者信息
type Subscriber struct {
	Topic   string
	Handler Handler
}

// Partition 分区结构
type partition struct {
	id      int
	queue   chan *Event
	ctx     context.Context
	cancel  context.CancelFunc
	handler Handler
}
