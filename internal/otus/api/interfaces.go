package api

import (
	"context"

	"github.com/google/gopacket"
)

type Source interface {
	Start(ctx context.Context) error
	ReadPacket() (data []byte, info gopacket.CaptureInfo, err error)
	Stop() error
}

type Decoder interface {
	Decode(data []byte, info gopacket.CaptureInfo) (*NetPacket, error)
}

type Processor interface {
	Process(exchange *Exchange) error
	Close() error
}

type FilterChain interface {
	Filter(exchange *Exchange)
}

type Filter interface {
	Filter(exchange *Exchange, chain FilterChain)
}

type Sink interface {
	Send(exchange *Exchange) error
	Close() error
}
