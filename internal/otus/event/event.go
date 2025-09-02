package event

import "fmt"

type EventContext struct {
	context map[string]interface{}
}

func GetValue[T any](event *EventContext, key string) (T, error) {
	v, ok := event.context[key]
	if !ok {
		return *new(T), fmt.Errorf("key not found")
	}
	t, ok := v.(T)
	if !ok {
		return *new(T), fmt.Errorf("unexpected value type")
	}
	return t, nil
}

func SetValue(event *EventContext, key string, value interface{}) {
	event.context[key] = value
}

func NewEventContext() *EventContext {
	return &EventContext{
		context: make(map[string]interface{}),
	}
}
