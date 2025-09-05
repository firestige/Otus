package event

import (
	"sync"

	"firestige.xyz/otus/internal/eventbus"
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

func Subscribe() {
	return bus.Subscribe()
}

func OnTransactionCreated(handle func()) {
	topic :=
		bus.Subscribe(eventbus.EventTypeTransactionCreated, func(event eventbus.Event) {
			handle()
		})
}
