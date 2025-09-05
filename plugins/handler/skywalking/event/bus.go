package event

import (
	"fmt"
	"sync"

	"firestige.xyz/otus/internal/eventbus"
	processor "firestige.xyz/otus/internal/otus/module/processor/api"
)

var (
	bus  eventbus.EventBus
	once *sync.Once
)

func Init(partitionCount, queueSize int) eventbus.EventBus {
	once.Do(func() {
		bus = eventbus.NewInMemoryEventBus(partitionCount, queueSize)
	})
	return bus
}

func Publish(event *eventbus.Event) error {
	return bus.Publish(event)
}

func Subscribe(topic string, handle processor.HandleFunc) {
	bus.Subscribe(topic, warp(handle))
}

func warp(handle processor.HandleFunc) eventbus.Handler {
	return func(event *eventbus.Event) error {
		ex, ok := event.Payload.(processor.Exchange)
		if !ok {
			return fmt.Errorf("invalid payload type")
		}
		handle(&ex)
		return nil
	}
}
