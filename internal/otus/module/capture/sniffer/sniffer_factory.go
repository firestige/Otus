package sniffer

import "firestige.xyz/otus/internal/otus/module/capture/codec"

func NewSniffer(options *Options) *Sniffer {
	return &Sniffer{
		options: options,
	}
}

func (s *Sniffer) SetDecoder(d *codec.Decoder) {
	s.decoder = d
}
