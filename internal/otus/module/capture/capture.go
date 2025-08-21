package capture

import (
	"context"
	"sync"

	otus "firestige.xyz/otus/internal/otus/api"
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/otus/module/capture/sniffer"
)

type Capture struct {
	SnifferOpt *sniffer.Options `mapstructure:"sniffer"`
	CodecOpt   *codec.Options   `mapstructure:"codec"`

	sniffer     *sniffer.Sniffer
	decoder     *codec.Decoder
	packetQueue chan *otus.NetPacket

	wg  *sync.WaitGroup
	ctx context.Context
}

func (c *Capture) PostConstruct() error {

	c.wg = &sync.WaitGroup{}
	return nil
}

func (c *Capture) Boot(ctx context.Context) {
	if c.sniffer == nil {
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.sniffer.Start(c.ctx); err != nil {
			// Handle error (e.g., log it)
		}
	}()
}

func (c *Capture) Shutdown() {
	c.wg.Wait()
	c.sniffer.Stop()
}
