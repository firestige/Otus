package sniffer

import (
	"context"

	"firestige.xyz/otus/internal/otus/capture/codec"
)

type Sniffer struct {
	NetInterface any
	Snaplen      int
	LocalIP      string
}

func NewSniffer(decoder *codec.Decoder, options *Options, ctx context.Context) *Sniffer {
}

func (s *Sniffer) Start(ctx context.Context) error {
}

func (s *Sniffer) Stop() {
}
