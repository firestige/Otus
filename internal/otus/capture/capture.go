package capture

import (
	"context"
	"sync"

	"firestige.xyz/otus/internal/otus/capture/codec"
	"firestige.xyz/otus/internal/otus/capture/sniffer"
)

type Capture struct {
	SnifferOpt *sniffer.Options `mapstructure:"sniffer"`
	CodecOpt   *codec.Options   `mapstructure:"codec"`

	sniffer *sniffer.Sniffer
	decoder *codec.Decoder

	wg  *sync.WaitGroup
	ctx context.Context
}

func (c *Capture) ConfigSpec() interface{} {
	return c
}

func (c *Capture) PostConfig(cfg interface{}, ctx context.Context) error {
	c.ctx = ctx
	c.decoder = codec.NewDecoder(ctx)
	c.sniffer = sniffer.NewSniffer(c.decoder, ctx)

	c.wg = &sync.WaitGroup{}
	return nil
}

func (c *Capture) Start() error {
	if c.sniffer == nil {
		return nil // Sniffer not configured
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.sniffer.Start(c.ctx); err != nil {
			// Handle error (e.g., log it)
		}
	}()

	return nil
}

func (c *Capture) Stop() {
}
