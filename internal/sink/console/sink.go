package console

import (
	"fmt"

	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/factory"
)

const Name = "console"

type Sink struct {
}

func NewSink() *Sink {
	return &Sink{}
}

func init() {
	fn := func(cfg interface{}) interface{} {
		s := NewSink()
		return s
	}
	factory.Register(otus.ComponentTypeSink, Name, fn)
}

func (s *Sink) Send(exchange *otus.Exchange) error {
	// 在控制台打印数据包信息
	fmt.Println("Received packet:", exchange)
	return nil
}

func (s *Sink) Close() error {
	return nil
}
