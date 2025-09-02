package eventbus

import (
	"firestige.xyz/otus/plugins/filter/skywalking/types"
)

// SipEvent SIP事件结构
type SipEvent struct {
	SipMsg *types.SipMessage `json:"sip_msg"`
	CallID string            `json:"call_id"`
}

// SipEventBus SIP事件总线封装
type SipEventBus struct {
	bus EventBus
}

// NewSipEventBus 创建SIP事件总线
func NewSipEventBus(partitionCount, queueSize int) *SipEventBus {
	return &SipEventBus{
		bus: NewInMemoryEventBus(partitionCount, queueSize),
	}
}

// PublishSipMessage 发布SIP消息事件
func (s *SipEventBus) PublishSipMessage(sipMsg *types.SipMessage) error {
	event := &Event{
		Topic:  "sip_message",
		CallID: sipMsg.CallID, // 使用SIP消息中的CallID作为分区键
		Ctx:    &SipEvent{SipMsg: sipMsg, CallID: sipMsg.CallID},
	}
	return s.bus.Publish(event)
}

// SubscribeSipMessages 订阅SIP消息
func (s *SipEventBus) SubscribeSipMessages(handler func(*types.SipMessage) error) error {
	return s.bus.Subscribe("sip_message", func(event *Event) error {
		sipEvent, ok := event.Ctx.(*SipEvent)
		if !ok {
			return nil
		}
		return handler(sipEvent.SipMsg)
	})
}

// Close 关闭事件总线
func (s *SipEventBus) Close() error {
	return s.bus.Close()
}

// GetStats 获取统计信息
func (s *SipEventBus) GetStats() *Stats {
	return s.bus.GetStats()
}
