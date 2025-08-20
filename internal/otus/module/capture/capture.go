package capture

import (
	"context"
	"sync"

	"firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/otus/module/capture/sniffer"
)

type Capture struct {
	SnifferOpt *sniffer.Options `mapstructure:"sniffer"`
	CodecOpt   *codec.Options   `mapstructure:"codec"`

	sniffer     *sniffer.Sniffer
	decoder     *codec.Decoder
	packetQueue chan *api.NetPacket

	wg  *sync.WaitGroup
	ctx context.Context
}

func NewCapture() Capture {
	return Capture{
		SnifferOpt:  &sniffer.Options{},
		CodecOpt:    &codec.Options{},
		packetQueue: make(chan *api.NetPacket, 100),
	}
}

func (c *Capture) ConfigSpec() interface{} {
	return c
}

func (c *Capture) PostConfig(cfg interface{}, ctx context.Context) error {
	c.ctx = ctx
	c.decoder = codec.NewDecoder(c.packetQueue, c.CodecOpt, ctx)
	c.sniffer = sniffer.NewSniffer(c.decoder, c.SnifferOpt, ctx)

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
	c.wg.Wait()
	c.sniffer.Stop()
}
