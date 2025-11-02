package api

import "context"

type Source interface {
	Start(ctx context.Context) error
	Stop() error
}

type Processor interface {
	Process(exchange *Exchange) error
}

type Sink interface {
	Send(exchange *Exchange) error
	Close() error
}
